import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

interface TuningField {
  key: string
  label: string
  min?: number
}

const commonFields: TuningField[] = [
  { key: 'SignedURLTTLSeconds', label: 'settings.signedURLTTLSeconds' },
  { key: 'PresignedPutTTLSeconds', label: 'settings.presignedPutTTLSeconds' },
  { key: 'MaxUploadsPerUser', label: 'settings.maxUploadsPerUser' },
]

const imageUploadFields: TuningField[] = [
  { key: 'VisionUploadMaxBytes', label: 'settings.visionUploadMaxBytes' },
  { key: 'VisionInlineMaxBytes', label: 'settings.visionInlineMaxBytes' },
  {
    key: 'UploadRetentionHours',
    label: 'settings.uploadRetentionHours',
    min: 0,
  },
]

const imageGenerationFields: TuningField[] = [
  {
    key: 'ImageGenRetentionHours',
    label: 'settings.imageGenRetentionHours',
    min: 0,
  },
  {
    key: 'ImageGenHistoryDays',
    label: 'settings.imageGenHistoryDays',
    min: 0,
  },
  {
    key: 'MaxImagesPerUser',
    label: 'settings.maxImagesPerUser',
    min: 0,
  },
  {
    key: 'ImageGenInlineMaxBytes',
    label: 'settings.imageGenInlineMaxBytes',
    min: 0,
  },
]

const videoGenerationFields: TuningField[] = [
  {
    key: 'VideoGenMaxBytes',
    label: 'videogen.settings.VideoGenMaxBytes',
    min: 1,
  },
  {
    key: 'VideoGenRetentionHours',
    label: 'videogen.settings.VideoGenRetentionHours',
    min: 0,
  },
  {
    key: 'VideoGenHistoryDays',
    label: 'videogen.settings.VideoGenHistoryDays',
    min: 0,
  },
  {
    key: 'MaxVideosPerUser',
    label: 'videogen.settings.MaxVideosPerUser',
    min: 0,
  },
]

const audioGenerationFields: TuningField[] = [
  {
    key: 'AudioGenConcurrency',
    label: 'audiogen.settings.AudioGenConcurrency',
    min: 1,
  },
  {
    key: 'AudioGenInputMaxBytes',
    label: 'audiogen.settings.AudioGenInputMaxBytes',
    min: 1,
  },
  {
    key: 'AudioGenMaxBytes',
    label: 'audiogen.settings.AudioGenMaxBytes',
    min: 1,
  },
  {
    key: 'AudioGenInlineMaxBytes',
    label: 'audiogen.settings.AudioGenInlineMaxBytes',
    min: 0,
  },
  {
    key: 'AudioGenRetentionHours',
    label: 'audiogen.settings.AudioGenRetentionHours',
    min: 0,
  },
  {
    key: 'AudioGenHistoryDays',
    label: 'audiogen.settings.AudioGenHistoryDays',
    min: 0,
  },
]

const categories = [
  {
    value: 'common',
    label: 'settings.tuningCommon',
    description: 'settings.tuningCommonDesc',
  },
  {
    value: 'image-upload',
    label: 'settings.tuningImageUpload',
    description: 'settings.tuningImageUploadDesc',
  },
  {
    value: 'image-generation',
    label: 'settings.tuningImageGeneration',
    description: 'settings.tuningImageGenerationDesc',
  },
  {
    value: 'video-generation',
    label: 'settings.tuningVideoGeneration',
    description: 'settings.tuningVideoGenerationDesc',
  },
  {
    value: 'audio-generation',
    label: 'settings.tuningAudioGeneration',
    description: 'settings.tuningAudioGenerationDesc',
  },
] as const

interface StorageTuningSettingsProps {
  values: Record<string, string>
  onChange: (key: string, value: string) => void
  onSave: (key: string) => void
  onToggle: (key: string) => void
}

export function StorageTuningSettings({
  values,
  onChange,
  onSave,
  onToggle,
}: StorageTuningSettingsProps) {
  const { t } = useTranslation()

  const fields = (items: TuningField[]) => (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
      {items.map(({ key, label, min }) => (
        <div key={key} className="min-w-0 space-y-2">
          <label htmlFor={`setting-${key}`} className="text-sm font-medium">
            {t(label)}
          </label>
          <Input
            id={`setting-${key}`}
            type="number"
            min={min}
            value={values[key] ?? ''}
            onChange={(e) => onChange(key, e.target.value)}
            onBlur={() => onSave(key)}
          />
        </div>
      ))}
    </div>
  )

  return (
    <div className="min-w-0 space-y-4 rounded-xl border bg-card p-5">
      <div className="space-y-1">
        <h2 className="text-sm font-semibold">{t('settings.uploadTuning')}</h2>
        <p className="text-xs text-muted-foreground">
          {t('settings.uploadTuningDesc')}
        </p>
      </div>
      <Tabs defaultValue="common" className="min-w-0">
        <TabsList
          aria-label={t('settings.uploadTuning')}
          className="grid h-auto w-full grid-cols-2 gap-1 sm:grid-cols-5"
        >
          {categories.map(({ value, label }) => (
            <TabsTrigger
              key={value}
              value={value}
              className={`h-full whitespace-normal px-2 py-1.5 text-xs sm:text-sm ${value === 'common' ? 'col-span-2 sm:col-span-1' : ''}`}
            >
              {t(label)}
            </TabsTrigger>
          ))}
        </TabsList>
        {categories.map(({ value, description }) => (
          <TabsContent key={value} value={value} className="space-y-4">
            <p className="text-xs leading-relaxed text-muted-foreground">
              {t(description)}
            </p>
            {value === 'common' && fields(commonFields)}
            {value === 'image-upload' && (
              <>
                {fields(imageUploadFields)}
                <div className="space-y-4 border-t pt-4">
                  <h3 className="text-sm font-medium">
                    {t('settings.tuningImageProcessing')}
                  </h3>
                  <div className="flex items-center justify-between gap-4">
                    <div className="space-y-0.5">
                      <label
                        htmlFor="setting-VisionResizeEnabled"
                        className="text-sm font-medium"
                      >
                        {t('settings.visionResizeEnabled')}
                      </label>
                      <p className="text-xs text-muted-foreground">
                        {t('settings.visionResizeDesc')}
                      </p>
                    </div>
                    <Switch
                      id="setting-VisionResizeEnabled"
                      className="shrink-0"
                      checked={values.VisionResizeEnabled === 'true'}
                      onCheckedChange={() => onToggle('VisionResizeEnabled')}
                    />
                  </div>
                  {fields([
                    {
                      key: 'VisionResizeMaxEdge',
                      label: 'settings.visionResizeMaxEdge',
                    },
                  ])}
                  <div className="flex items-center justify-between gap-4">
                    <div className="space-y-0.5">
                      <label
                        htmlFor="setting-VisionCompressEnabled"
                        className="text-sm font-medium"
                      >
                        {t('settings.visionCompressEnabled')}
                      </label>
                      <p className="text-xs text-muted-foreground">
                        {t('settings.visionCompressDesc')}
                      </p>
                    </div>
                    <Switch
                      id="setting-VisionCompressEnabled"
                      className="shrink-0"
                      checked={values.VisionCompressEnabled === 'true'}
                      onCheckedChange={() => onToggle('VisionCompressEnabled')}
                    />
                  </div>
                  {fields([
                    {
                      key: 'VisionJPEGQuality',
                      label: 'settings.visionJPEGQuality',
                    },
                  ])}
                </div>
              </>
            )}
            {value === 'image-generation' && fields(imageGenerationFields)}
            {value === 'video-generation' && fields(videoGenerationFields)}
            {value === 'audio-generation' && fields(audioGenerationFields)}
          </TabsContent>
        ))}
      </Tabs>
    </div>
  )
}
