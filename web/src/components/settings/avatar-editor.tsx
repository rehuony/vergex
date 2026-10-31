import { useEffect, useRef, useState, type ChangeEvent } from 'react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { Camera, ImageUp, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { useAuth } from '@/contexts/auth-context'
import { useProfile } from '@/hooks/use-profile'
import { t, type Language } from '@/i18n/translations'
import { api } from '@/api'
import { AVATAR_TYPES, checkAvatarFile, prepareAvatarSource } from './avatar'
import { UserAvatar } from '@/components/common/user-avatar'
import { Button } from '@/components/ui/button'
import { AvatarCropDialog } from '@/components/settings/avatar-crop-dialog'

const menuItemClass =
  'flex h-9 cursor-pointer select-none items-center gap-2 rounded-lg px-2.5 text-sm text-vergex-text outline-none data-[highlighted]:bg-vergex-bg'

/**
 * The avatar with an edit badge on hover. Picking an image opens the photo
 * editor; the crop is redrawn in the browser, and again by the server, before
 * it is stored.
 */
export function AvatarEditor({ language }: { language: Language }) {
  const { user } = useAuth()
  const { data: profile, mutate } = useProfile()
  const fileRef = useRef<HTMLInputElement>(null)
  const [src, setSrc] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // Release the picked file once its cropper closes.
  useEffect(
    () => () => {
      if (src) URL.revokeObjectURL(src)
    },
    [src]
  )

  const pick = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const problem = checkAvatarFile(file)
    if (problem) {
      toast.error(t(problem, language))
      return
    }
    try {
      setSrc(await prepareAvatarSource(file))
    } catch {
      toast.error(t('avatarUnreadable', language))
    }
  }

  const save = async (image: Blob) => {
    try {
      const avatar = await api.updateAvatar(image)
      await mutate(profile && { ...profile, avatar }, { revalidate: false })
      toast.success(t('avatarUpdated', language))
      setSrc(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    }
  }

  const remove = async () => {
    setBusy(true)
    try {
      await api.removeAvatar()
      await mutate(profile && { ...profile, avatar: '' }, {
        revalidate: false,
      })
      toast.success(t('avatarRemoved', language))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const choose = () => fileRef.current?.click()

  const trigger = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      disabled={busy}
      aria-label={t('editAvatar', language)}
      className="group relative size-14 rounded-full p-0 hover:bg-transparent"
      onClick={profile?.avatar ? undefined : choose}
    >
      <UserAvatar
        email={user?.email ?? ''}
        avatar={profile?.avatar}
        className="size-14 text-lg"
      />
      {/* Hidden until hover or focus; always shown where there is no hover. */}
      <span className="absolute inset-0 grid place-items-center rounded-full bg-black/45 text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100 group-data-[state=open]:opacity-100 [@media(hover:none)]:opacity-100">
        <Camera className="size-5" aria-hidden="true" />
      </span>
    </Button>
  )

  return (
    <>
      <input
        ref={fileRef}
        type="file"
        accept={AVATAR_TYPES.join(',')}
        className="hidden"
        tabIndex={-1}
        onChange={(e) => void pick(e)}
      />
      {profile?.avatar ? (
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content
              align="start"
              sideOffset={6}
              className="z-[60] min-w-44 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-1.5 shadow-[var(--shadow-lg)]"
            >
              <DropdownMenu.Item className={menuItemClass} onSelect={choose}>
                <ImageUp className="size-4" aria-hidden="true" />
                {t('uploadAvatar', language)}
              </DropdownMenu.Item>
              <DropdownMenu.Item
                className={`${menuItemClass} text-vergex-danger`}
                onSelect={() => void remove()}
              >
                <Trash2 className="size-4" aria-hidden="true" />
                {t('removeAvatar', language)}
              </DropdownMenu.Item>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
      ) : (
        trigger
      )}
      {src && (
        <AvatarCropDialog
          src={src}
          language={language}
          onCancel={() => setSrc(null)}
          onSave={save}
        />
      )}
    </>
  )
}
