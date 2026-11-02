import React, {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
} from 'react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { buttonVariants } from '@/components/ui/button'
import { activeLanguage, t } from '@/i18n'
import {
  setGlobalConfirm,
  type ConfirmOptions as NotifyConfirmOptions,
} from './notify'

type ConfirmOptions = NotifyConfirmOptions & { message: string }

interface ConfirmDialogContextType {
  confirm: (options: ConfirmOptions) => Promise<boolean>
}

const ConfirmDialogContext = createContext<
  ConfirmDialogContextType | undefined
>(undefined)

export function useConfirmDialog() {
  const context = useContext(ConfirmDialogContext)
  if (!context) {
    throw new Error(
      'useConfirmDialog must be used within ConfirmDialogProvider'
    )
  }
  return context
}

interface ConfirmState extends ConfirmOptions {
  isOpen: boolean
  resolve?: (value: boolean) => void
}

export function ConfirmDialogProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [state, setState] = useState<ConfirmState>({
    isOpen: false,
    title: '',
    message: '',
    okText: '',
  })

  const confirm = useCallback((options: ConfirmOptions): Promise<boolean> => {
    return new Promise((resolve) => {
      setState({ ...options, isOpen: true, resolve })
    })
  }, [])

  // Register global confirm function
  useEffect(() => {
    setGlobalConfirm(confirm)
  }, [confirm])

  const handleClose = useCallback((result: boolean) => {
    setState((prev) => {
      prev.resolve?.(result)
      return {
        ...prev,
        isOpen: false,
      }
    })
  }, [])

  return (
    <ConfirmDialogContext.Provider value={{ confirm }}>
      {children}
      <AlertDialog
        open={state.isOpen}
        onOpenChange={(open) => !open && handleClose(false)}
      >
        <AlertDialogContent>
          <AlertDialogTitle>{state.title}</AlertDialogTitle>
          <AlertDialogDescription className="mt-1 whitespace-pre-line">
            {state.message}
          </AlertDialogDescription>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => handleClose(false)}>
              {state.cancelText ?? t('cancel', activeLanguage())}
            </AlertDialogCancel>
            <AlertDialogAction
              className={
                state.tone === 'danger'
                  ? buttonVariants({ variant: 'destructive' })
                  : undefined
              }
              onClick={() => handleClose(true)}
            >
              {state.okText}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </ConfirmDialogContext.Provider>
  )
}
