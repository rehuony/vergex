import { useEffect, useRef, useState, type ReactNode } from 'react'
import ReactCrop, {
  centerCrop,
  makeAspectCrop,
  type PercentCrop,
} from 'react-image-crop'
import 'react-image-crop/dist/ReactCrop.css'
import './avatar-crop-dialog.css'
import { RotateCcwSquare, RotateCwSquare, Undo2 } from 'lucide-react'
import { t, type Language } from '@/i18n'
import { renderAvatar, rotateQuarter } from '@/components/settings/avatar'
import { Dialog } from '@/components/common/dialog'
import { Button } from '@/components/ui/button'

// Both steps keep the room of the stage and the rotate tiles, so the dialog does not change
// size between them.
const stage = 'grid h-[min(360px,50dvh)] w-full place-items-center'
const controls = 'flex h-[72px] items-center justify-center'

// The largest centred square, which is where Google's editor starts too.
function fullSquare(width: number, height: number): PercentCrop {
  const side = width > height ? { height: 100 } : { width: 100 }
  return centerCrop(
    makeAspectCrop({ unit: '%', ...side }, 1, width, height),
    width,
    height
  )
}

function RotateTile({
  icon,
  label,
  onClick,
}: {
  icon: ReactNode
  label: string
  onClick: () => void
}) {
  return (
    <Button
      type="button"
      variant="ghost"
      className="h-auto w-22 flex-col gap-1.5 rounded-2xl bg-vergex-surface-muted px-1 py-3 font-normal"
      onClick={onClick}
    >
      {icon}
      {label}
    </Button>
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
  const [saving, setSaving] = useState(false)
  // Every control stays clickable. Clicks on the photo run one after another, so
  // a quick second one is not lost and starts from what the first one made.
  const latest = useRef(src)
  const pending = useRef(0)
  const queue = useRef<Promise<void>>(Promise.resolve())

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

  const enqueue = (step: () => void | Promise<void>) => {
    pending.current += 1
    queue.current = queue.current
      .then(step)
      .catch((error) => console.error('Could not change the photo:', error))
      .finally(() => {
        pending.current -= 1
      })
  }

  // The frame starts over: the new orientation opens on the full square.
  const rotate = (direction: 'left' | 'right') =>
    enqueue(async () => {
      const rotated = await rotateQuarter(latest.current, direction)
      latest.current = rotated
      setCrop(undefined)
      setImage(rotated)
    })

  const reset = () =>
    enqueue(() => {
      if (latest.current !== src) {
        latest.current = src
        setCrop(undefined)
        setImage(src)
        return
      }
      const element = imageRef.current
      if (element) {
        setCrop(fullSquare(element.naturalWidth, element.naturalHeight))
      }
    })

  // While a turn is running or the photo is loading there is nothing to crop yet.
  const next = () => {
    const element = imageRef.current
    if (pending.current > 0 || !element || !crop?.width) return
    enqueue(async () => {
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
  }

  const save = async (blob: Blob) => {
    if (saving) return
    setSaving(true)
    try {
      await onSave(blob)
    } finally {
      setSaving(false)
    }
  }

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
      footer={
        preview ? (
          <Button
            type="button"
            variant="primary"
            aria-busy={saving}
            onClick={() => void save(preview.blob)}
          >
            {saving ? t('saving', language) : t('saveAsAvatar', language)}
          </Button>
        ) : (
          <Button type="button" variant="primary" onClick={next}>
            {t('next', language)}
          </Button>
        )
      }
    >
      {preview ? (
        <>
          <div className={stage}>
            <img
              src={preview.url}
              alt={t('avatarPreview', language)}
              draggable={false}
              className="size-48 select-none rounded-full"
            />
          </div>
          <div className={controls} />
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
                draggable={false}
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
            <div className="flex gap-2">
              <RotateTile
                icon={<RotateCcwSquare className="size-5" aria-hidden="true" />}
                label={t('rotateLeft', language)}
                onClick={() => rotate('left')}
              />
              <RotateTile
                icon={<Undo2 className="size-5" aria-hidden="true" />}
                label={t('rotateReset', language)}
                onClick={reset}
              />
              <RotateTile
                icon={<RotateCwSquare className="size-5" aria-hidden="true" />}
                label={t('rotateRight', language)}
                onClick={() => rotate('right')}
              />
            </div>
          </div>
        </>
      )}
    </Dialog>
  )
}
