package zhipu

import (
	"bufio"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	openaicompat "github.com/QuantumNous/new-api/relay/channel/openai_compat"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func zhipuResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var zhipuResponse ZhipuResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	if err := common.Unmarshal(responseBody, &zhipuResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if !zhipuResponse.Success {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResponse.Msg,
			Code:    zhipuResponse.Code,
		}, resp.StatusCode)
	}
	fullTextResponse := responseZhipu2OpenAI(&zhipuResponse)
	fullTextResponse.Model = info.UpstreamModelName
	if _, newAPIError := openaicompat.WriteChatCompletionsResponseAsResponses(c, info, resp, fullTextResponse); newAPIError != nil {
		return nil, newAPIError
	}
	return &fullTextResponse.Usage, nil
}

func zhipuResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var usage *dto.Usage
	converter := openaicompat.NewResponsesStreamConverter(c, info)
	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	dataChan := make(chan string)
	metaChan := make(chan string)
	stopChan := make(chan bool)

	go func() {
		for scanner.Scan() {
			data := scanner.Text()
			lines := strings.Split(data, "\n")
			for i, line := range lines {
				if len(line) < 5 {
					continue
				}
				if line[:5] == "data:" {
					dataChan <- line[5:]
					if i != len(lines)-1 {
						dataChan <- "\n"
					}
				} else if line[:5] == "meta:" {
					metaChan <- line[5:]
				}
			}
		}
		if err := scanner.Err(); err != nil {
			common.SysLog("error reading stream: " + err.Error())
		}
		stopChan <- true
	}()

	helper.SetEventStreamHeaders(c)
	c.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			response := streamResponseZhipu2OpenAI(data)
			response.Model = info.UpstreamModelName
			converter.HandleChatChunk(response)
			return true
		case data := <-metaChan:
			var zhipuResponse ZhipuStreamMetaResponse
			if err := common.UnmarshalJsonStr(data, &zhipuResponse); err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				return true
			}
			response, zhipuUsage := streamMetaResponseZhipu2OpenAI(&zhipuResponse)
			response.Model = info.UpstreamModelName
			usage = zhipuUsage
			converter.SetUsage(usage)
			converter.HandleChatChunk(response)
			return true
		case <-stopChan:
			if usage != nil {
				converter.SetUsage(usage)
			}
			finalUsage := converter.Finish()
			if usage == nil {
				usage = finalUsage
			}
			return false
		}
	})
	service.CloseResponseBodyGracefully(resp)
	if usage == nil {
		usage = &dto.Usage{}
	}
	return usage, nil
}
