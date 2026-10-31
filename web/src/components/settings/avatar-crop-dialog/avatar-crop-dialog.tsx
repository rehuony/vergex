import { useEffect, useRef, useState } from 'react'
import ReactCrop, {
  centerCrop,
  makeAspectCrop,
  type PercentCrop,
} from 'react-image-crop'
import 'react-image-crop/dist/ReactCrop.css'
import './avatar-crop-dialog.css'
import { RotateCcwSquare } from 'lucide-react'
import { t, type Language } from '@/i18n/translations'
import { renderAvatar, rotateLeft } from '@/components/settings/avatar'
import { Dialog } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'

// Both steps share the stage and the controls' heights (the rotate tile, the
// gap and the pill), so the dialog does not change size between them.
const stage = 'grid h-[min(360px,50dvh)] w-full place-items-center'
const controls = 'flex h-[130px] flex-col items-center justify-end gap-5'

// The largest centred square, which is where Google's editor starts too.
function fullSquare(width: number, height: number): PercentCrop {
  const side = width > height ? { height: 100 } : { width: 100 }
  return centerCrop(
    makeAspectCrop({ unit: '%', ...side }, 1, width, height),
    width,
    height
  )
}

/**
 * Google-style photo editor: crop and rotate, then preview and save. `src` is
 * owned by the caller; rotated copies made here are released here.
 */
export function AvatarCropDialog({
  src,
  language,
  onCancel,
  onSave,
}: {
  src: string
  language: Language
  onCancel: () => void
  onSave: (image: Blob) => Promise<void>
}) {
  const imageRef = useRef<HTMLImageElement>(null)
  const [image, setImage] = useState(src)
  const [crop, setCrop] = useState<PercentCrop>()
  const [preview, setPreview] = useState<{ blob: Blob; url: string } | null>(
    null
  )
  const [busy, setBusy] = useState(false)

  useEffect(
    () => () => {
      if (image !== src) URL.revokeObjectURL(image)
    },
    [image, src]
  )
  useEffect(
    () => () => {
      if (preview) URL.revokeObjectURL(preview.url)
    },
    [preview]
  )

  const run = async (action: () => Promise<void>) => {
    setBusy(true)
    try {
      await action()
    } finally {
      setBusy(false)
    }
  }

  const next = () =>
    run(async () => {
      const element = imageRef.current
      if (!element || !crop) return
      const width = element.naturalWidth
      const height = element.naturalHeight
      const blob = await renderAvatar(image, {
        x: (crop.x / 100) * width,
        y: (crop.y / 100) * height,
        width: (crop.width / 100) * width,
        height: (crop.height / 100) * height,
      })
      setPreview({ blob, url: URL.createObjectURL(blob) })
    })

  return (
    <Dialog
      title={
        <span className="block text-center">
          {t(preview ? 'avatarPreview' : 'cropAndRotate', language)}
        </span>
      }
      size="md"
      onClose={onCancel}
      onBack={preview ? () => setPreview(null) : onCancel}
      bodyClassName="flex min-h-0 flex-1 flex-col items-center gap-5 overflow-y-auto px-6 py-6"
    >
      {preview ? (
        <>
          <div className={stage}>
            <img
              src={preview.url}
              alt={t('avatarPreview', language)}
              className="size-48 rounded-full"
            />
          </div>
          <div className={controls}>
            <Button
              type="button"
              variant="primary"
              className="rounded-full px-6"
              disabled={busy}
              onClick={() => void run(() => onSave(preview.blob))}
            >
              {busy ? t('saving', language) : t('saveAsAvatar', language)}
            </Button>
          </div>
        </>
      ) : (
        <>
          <div className={stage}>
            <ReactCrop
              className="vergex-avatar-crop max-h-[min(360px,50dvh)]"
              crop={crop}
              onChange={(_, percent) => setCrop(percent)}
              aspect={1}
              minWidth={48}
              keepSelection
              renderSelectionAddon={() => (
                <span className="vergex-avatar-crop-circle" />
              )}
            >
              <img
                ref={imageRef}
                src={image}
                alt=""
                // Keep the frame when coming back from the preview; rotating
                // clears it so the new orientation starts from the full square.
                onLoad={(e) => {
                  const { naturalWidth, naturalHeight } = e.currentTarget
                  setCrop(
                    (current) =>
                      current ?? fullSquare(naturalWidth, naturalHeight)
                  )
                }}
              />
            </ReactCrop>
          </div>
          <div className={controls}>
            <Button
              type="button"
              variant="ghost"
              className="h-auto flex-col gap-1.5 rounded-xl bg-vergex-bg px-5 py-3 font-normal"
              disabled={busy}
              onClick={() =>
                void run(async () => {
                  const rotated = await rotateLeft(image)
                  setCrop(undefined)
                  setImage(rotated)
                })
              }
            >
              <RotateCcwSquare className="size-5" aria-hidden="true" />
              {t('rotate', language)}
            </Button>
            <Button
              type="button"
              variant="primary"
              className="rounded-full px-8"
              disabled={busy || !crop?.width}
              onClick={() => void next()}
            >
              {t('next', language)}
            </Button>
          </div>
        </>
      )}
    </Dialog>
  )
}
