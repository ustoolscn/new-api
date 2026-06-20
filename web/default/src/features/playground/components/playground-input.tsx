/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useRef, useState, type ChangeEvent } from 'react'
import {
  ImageIcon,
  SendIcon,
  SquareIcon,
  Loader2,
  XIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  PromptInput,
  PromptInputButton,
  PromptInputFooter,
  PromptInputTextarea,
  PromptInputTools,
  type PromptInputMessage,
} from '@/components/ai-elements/prompt-input'
import { ModelGroupSelector } from '@/components/model-group-selector'
import { uploadPlaygroundImage } from '../api'
import type { ModelOption, GroupOption } from '../types'

interface PlaygroundInputProps {
  onSubmit: (text: string, imageUrls?: string[]) => void
  onStop?: () => void
  disabled?: boolean
  isGenerating?: boolean
  models: ModelOption[]
  modelValue: string
  onModelChange: (value: string) => void
  isModelLoading?: boolean
  groups: GroupOption[]
  groupValue: string
  onGroupChange: (value: string) => void
}

interface UploadedImage {
  id: string
  name: string
  url: string
  thumbnailUrl?: string
}

export function PlaygroundInput({
  onSubmit,
  onStop,
  disabled,
  isGenerating,
  models,
  modelValue,
  onModelChange,
  isModelLoading = false,
  groups,
  groupValue,
  onGroupChange,
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [uploadedImages, setUploadedImages] = useState<UploadedImage[]>([])
  const [isUploadingImage, setIsUploadingImage] = useState(false)
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  const isModelSelectDisabled =
    disabled || isModelLoading || models.length === 0
  const isGroupSelectDisabled = disabled || groups.length === 0

  const handleSubmit = (message: PromptInputMessage) => {
    const messageText = message.text?.trim() || ''
    if ((!messageText && uploadedImages.length === 0) || disabled) return
    onSubmit(
      messageText,
      uploadedImages.map((image) => image.url)
    )
    setText('')
    setUploadedImages([])
  }

  const handleImageUpload = async (
    event: ChangeEvent<HTMLInputElement>
  ) => {
    const files = Array.from(event.target.files || [])
    event.target.value = ''
    const imageFiles = files.filter((file) => file.type.startsWith('image/'))

    if (files.length > 0 && imageFiles.length === 0) {
      toast.error(t('No files match the accepted types.'))
      return
    }
    if (imageFiles.length === 0) return

    setIsUploadingImage(true)
    try {
      const uploaded = await Promise.all(
        imageFiles.map((file) => uploadPlaygroundImage(file))
      )
      setUploadedImages((prev) => [
        ...prev,
        ...uploaded.map((image, index) => ({
          id: `${Date.now()}-${index}-${image.url}`,
          name: image.filename || imageFiles[index]?.name || t('Image'),
          url: image.url,
          thumbnailUrl: image.thumbnail_url,
        })),
      ])
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setIsUploadingImage(false)
    }
  }

  return (
    <div className='grid shrink-0 px-1 md:pb-4'>
      <PromptInput groupClassName='rounded-xl' onSubmit={handleSubmit}>
        <PromptInputTextarea
          autoComplete='off'
          autoCorrect='off'
          autoCapitalize='off'
          spellCheck={false}
          className='px-5 md:text-base'
          disabled={disabled}
          onChange={(event) => setText(event.target.value)}
          placeholder={t('Ask anything')}
          value={text}
        />

        {uploadedImages.length > 0 && (
          <div className='flex gap-2 overflow-x-auto px-3 pb-2'>
            {uploadedImages.map((image) => (
              <div
                className='border-border bg-muted relative size-16 shrink-0 overflow-hidden rounded-lg border'
                key={image.id}
              >
                <img
                  alt={image.name || t('Image')}
                  className='size-full object-cover'
                  src={image.thumbnailUrl || image.url}
                />
                <button
                  aria-label={t('Remove attachment')}
                  className='bg-background/90 text-foreground absolute top-1 right-1 flex size-5 items-center justify-center rounded-full shadow-sm'
                  onClick={() =>
                    setUploadedImages((prev) =>
                      prev.filter((item) => item.id !== image.id)
                    )
                  }
                  type='button'
                >
                  <XIcon size={12} />
                </button>
              </div>
            ))}
          </div>
        )}

        <PromptInputFooter className='p-2.5'>
          <PromptInputTools>
            <input
              accept='image/*'
              className='sr-only'
              multiple
              onChange={handleImageUpload}
              ref={fileInputRef}
              type='file'
            />
            <PromptInputButton
              className='border font-medium'
              disabled={disabled || isUploadingImage}
              onClick={() => fileInputRef.current?.click()}
              type='button'
              variant='outline'
            >
              {isUploadingImage ? (
                <Loader2 className='animate-spin' size={16} />
              ) : (
                <ImageIcon size={16} />
              )}
              <span className='hidden sm:inline'>{t('Upload photo')}</span>
              <span className='sr-only sm:hidden'>{t('Upload photo')}</span>
            </PromptInputButton>
          </PromptInputTools>

          <div className='flex items-center gap-1.5 md:gap-2'>
            <ModelGroupSelector
              selectedModel={modelValue}
              models={models}
              onModelChange={onModelChange}
              selectedGroup={groupValue}
              groups={groups}
              onGroupChange={onGroupChange}
              disabled={isModelSelectDisabled || isGroupSelectDisabled}
            />

            {isGenerating && onStop ? (
              <PromptInputButton
                className='text-foreground font-medium'
                onClick={onStop}
                variant='secondary'
              >
                <SquareIcon className='fill-current' size={16} />
                <span className='hidden sm:inline'>{t('Stop')}</span>
                <span className='sr-only sm:hidden'>{t('Stop')}</span>
              </PromptInputButton>
            ) : (
              <PromptInputButton
                className='text-foreground font-medium'
                disabled={
                  disabled ||
                  isUploadingImage ||
                  (!text.trim() && uploadedImages.length === 0)
                }
                type='submit'
                variant='secondary'
              >
                <SendIcon size={16} />
                <span className='hidden sm:inline'>{t('Send')}</span>
                <span className='sr-only sm:hidden'>{t('Send')}</span>
              </PromptInputButton>
            )}
          </div>
        </PromptInputFooter>
      </PromptInput>
    </div>
  )
}
