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

interface ConfirmState {
  isOpen: boolean
  title?: string
  message: string
  okText: string
  cancelText: string
  tone?: 'danger'
  resolve?: (value: boolean) => void
}

export function ConfirmDialogProvider({
  children,
}: {
  children: React.ReactNode
}) {
  const [state, setState] = useState<ConfirmState>({
    isOpen: false,
    message: '',
    okText: 'Confirm',
    cancelText: 'Cancel',
  })

  const confirm = useCallback((options: ConfirmOptions): Promise<boolean> => {
    return new Promise((resolve) => {
      setState({
        isOpen: true,
        title: options.title,
        message: options.message,
        okText: options.okText || 'Confirm',
        cancelText: options.cancelText || 'Cancel',
        tone: options.tone,
        resolve,
      })
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
          <div className="flex flex-col gap-5 text-center">
            {state.title && (
              <AlertDialogTitle className="text-xl">
                {state.title}
              </AlertDialogTitle>
            )}
            <AlertDialogDescription className="text-(--text-primary) text-base font-medium">
              {state.message}
            </AlertDialogDescription>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => handleClose(false)}>
              {state.cancelText}
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
