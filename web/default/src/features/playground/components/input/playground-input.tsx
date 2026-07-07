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
import { ImageIcon, Loader2, XIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  PromptInput,
  PromptInputButton,
  PromptInputFooter,
  PromptInputTextarea,
  type PromptInputMessage,
} from '@/components/ai-elements/prompt-input'

import { uploadPlaygroundImage } from '../../api'
import type { ModelOption, GroupOption } from '../../types'
import { PlaygroundInputControls } from './playground-input-controls'
import { PlaygroundInputTools } from './playground-input-tools'

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
  hasMessages?: boolean
  onClearMessages?: () => void
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
  hasMessages = false,
  onClearMessages,
}: PlaygroundInputProps) {
  const { t } = useTranslation()
  const [text, setText] = useState('')
  const [uploadedImages, setUploadedImages] = useState<UploadedImage[]>([])
  const [isUploadingImage, setIsUploadingImage] = useState(false)
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  const handleSubmit = (message: PromptInputMessage) => {
    const messageText = message.text?.trim() || ''

    if (disabled || (!messageText && uploadedImages.length === 0)) return

    onSubmit(
      messageText,
      uploadedImages.map((image) => image.url)
    )
    setText('')
    setUploadedImages([])
  }

  const handleImageUpload = async (event: ChangeEvent<HTMLInputElement>) => {
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
      setUploadedImages((previousImages) => [
        ...previousImages,
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
    <div className='grid shrink-0 gap-4 px-1 md:pb-4'>
      <PromptInput
        className='relative'
        groupClassName='bg-background/95 dark:bg-background/80 border-border/70 shadow-[0_18px_60px_-32px_rgba(0,0,0,0.65)] ring-1 ring-foreground/5 rounded-xl overflow-hidden transition-all duration-200 focus-within:border-primary/45 focus-within:ring-primary/15 focus-within:shadow-[0_22px_70px_-34px_rgba(0,0,0,0.75)]'
        onSubmit={handleSubmit}
      >
        <input
          accept='image/*'
          className='sr-only'
          multiple
          onChange={handleImageUpload}
          ref={fileInputRef}
          type='file'
        />
        <PromptInputTextarea
          autoComplete='off'
          autoCorrect='off'
          autoCapitalize='off'
          spellCheck={false}
          className='min-h-20 px-5 pt-4 pb-3 leading-7 md:min-h-24 md:text-base'
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
                    setUploadedImages((previousImages) =>
                      previousImages.filter((item) => item.id !== image.id)
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

        <PromptInputFooter className='border-border/60 bg-muted/20 dark:bg-muted/10 border-t px-3 py-2.5 backdrop-blur'>
          <PlaygroundInputControls
            disabled={disabled}
            groups={groups}
            groupValue={groupValue}
            hasAttachments={uploadedImages.length > 0}
            isGenerating={isGenerating}
            isUploading={isUploadingImage}
            isModelLoading={isModelLoading}
            models={models}
            modelValue={modelValue}
            onGroupChange={onGroupChange}
            onModelChange={onModelChange}
            onStop={onStop}
            text={text}
            tools={
              <PlaygroundInputTools
                disabled={disabled}
                hasMessages={hasMessages}
                onClearMessages={onClearMessages}
                uploadAction={
                  <PromptInputButton
                    aria-label={t('Upload photo')}
                    className='text-muted-foreground hover:text-foreground hover:bg-muted/70 font-medium'
                    disabled={disabled || isUploadingImage}
                    onClick={() => fileInputRef.current?.click()}
                    variant='ghost'
                  >
                    {isUploadingImage ? (
                      <Loader2 className='animate-spin' size={16} />
                    ) : (
                      <ImageIcon size={16} />
                    )}
                  </PromptInputButton>
                }
              />
            }
          />
        </PromptInputFooter>
      </PromptInput>
    </div>
  )
}
