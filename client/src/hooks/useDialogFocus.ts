import { useEffect, useRef } from 'react'

const focusableSelector = 'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

export function useDialogFocus(onClose: () => void) {
    const dialogRef = useRef<HTMLElement>(null)
    const onCloseRef = useRef(onClose)

    useEffect(() => {
        onCloseRef.current = onClose
    }, [onClose])

    useEffect(() => {
        const dialog = dialogRef.current
        if (!dialog) return

        const previousFocus = document.activeElement as HTMLElement | null
        const focusable = () => Array.from(dialog.querySelectorAll<HTMLElement>(focusableSelector))
        const focusFirst = () => (focusable()[0] ?? dialog).focus()

        const handleKeyDown = (event: KeyboardEvent) => {
            if (event.key === 'Escape') {
                event.preventDefault()
                event.stopPropagation()
                onCloseRef.current()
                return
            }
            if (event.key !== 'Tab') return

            const elements = focusable()
            const first = elements[0] ?? dialog
            const last = elements[elements.length - 1] ?? dialog
            const active = document.activeElement
            if (!dialog.contains(active) || active === dialog || (event.shiftKey ? active === first : active === last)) {
                event.preventDefault()
                const next = event.shiftKey ? last : first
                next.focus()
            }
        }
        const handleFocusIn = () => {
            if (!dialog.contains(document.activeElement)) focusFirst()
        }

        document.addEventListener('keydown', handleKeyDown)
        document.addEventListener('focusin', handleFocusIn)
        focusFirst()

        return () => {
            document.removeEventListener('keydown', handleKeyDown)
            document.removeEventListener('focusin', handleFocusIn)
            if (previousFocus?.isConnected) previousFocus.focus()
        }
    }, [])

    return dialogRef
}
