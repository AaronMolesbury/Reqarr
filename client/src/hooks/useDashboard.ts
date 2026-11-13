import { useCallback, useEffect, useRef, useState } from 'react'
import type { DashboardResponse, MediaRequest } from '../types/request'
import type { Integrations } from '../types/integration'
import { parseIntegrations } from '../utils/integrations'

const AUTO_REFRESH_MS = 90_000
const RETRY_REFRESH_MS = 5_000
const MAX_CONSECUTIVE_FAILURES = 3

export function useDashboard() {
    const [requests, setRequests] = useState<MediaRequest[]>([])
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState('')
    const [loadedAt, setLoadedAt] = useState<Date | null>(null)
    const [integrations, setIntegrations] = useState<Integrations | null>(null)
    const [autoRefreshIn, setAutoRefreshIn] = useState<number | null>(AUTO_REFRESH_MS / 1000)
    const [autoRefreshStopped, setAutoRefreshStopped] = useState(false)

    const loadingRef = useRef(false)
    const nextRefreshAt = useRef(0)
    const failures = useRef(0)
    const activeController = useRef<AbortController | null>(null)

    const refresh = useCallback(async () => {
        // Ignore overlapping timer, visibility and manual requests.
        if (loadingRef.current) return
        // Once stopped, only an explicit manual refresh can reach this path.
        if (failures.current >= MAX_CONSECUTIVE_FAILURES) failures.current = 0
        loadingRef.current = true
        const controller = new AbortController()
        activeController.current = controller
        setLoading(true)
        setAutoRefreshStopped(false)
        let succeeded = false
        try {
            const response = await fetch('/api/dashboard', { signal: controller.signal, cache: 'no-store' })
            const data: DashboardResponse | null = await response.json().catch(() => null)
            if (controller.signal.aborted) return
            const results = parseIntegrations(data?.integrations)
            if (!response.ok) {
                if (results) setIntegrations(results)
                throw new Error(`API returned HTTP ${response.status}`)
            }
            if (!data || !Array.isArray(data.requests)) throw new Error('Unexpected response from API')
            setRequests(data.requests)
            setIntegrations(results)
            const now = Date.now()
            const checkedAt = data.checkedAt ? Date.parse(data.checkedAt) : NaN
            setLoadedAt(new Date(Number.isNaN(checkedAt) ? now : checkedAt))
            setError('')
            succeeded = true
        } catch (cause) {
            if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : 'Could not fetch requests')
        } finally {
            // An aborted request from an earlier effect must not unlock or
            // update a newer request (including React StrictMode's remount).
            if (activeController.current === controller) {
                activeController.current = null
                loadingRef.current = false
                if (!controller.signal.aborted) {
                    failures.current = succeeded ? 0 : failures.current + 1
                    const stopped = failures.current >= MAX_CONSECUTIVE_FAILURES
                    const retryDelay = RETRY_REFRESH_MS * 2 ** (failures.current - 1)
                    const delay = succeeded ? AUTO_REFRESH_MS : retryDelay
                    nextRefreshAt.current = stopped ? Infinity : Date.now() + delay
                    setAutoRefreshStopped(stopped)
                    setAutoRefreshIn(!stopped && document.visibilityState === 'visible' ? delay / 1000 : null)
                    setLoading(false)
                }
            }
        }
    }, [])

    useEffect(() => {
        void refresh()
        const tick = () => {
            if (document.visibilityState === 'visible' &&
                failures.current < MAX_CONSECUTIVE_FAILURES &&
                !loadingRef.current && Date.now() >= nextRefreshAt.current) {
                void refresh()
            }
            setAutoRefreshIn(document.visibilityState === 'visible' && failures.current < MAX_CONSECUTIVE_FAILURES
                ? Math.max(0, Math.ceil((nextRefreshAt.current - Date.now()) / 1000))
                : null)
        }
        const timer = window.setInterval(tick, 1000)
        document.addEventListener('visibilitychange', tick)
        return () => {
            window.clearInterval(timer)
            document.removeEventListener('visibilitychange', tick)
            activeController.current?.abort()
            activeController.current = null
            loadingRef.current = false
        }
    }, [refresh])

    return { requests, loading, error, loadedAt, refresh, integrations, autoRefreshIn, autoRefreshStopped }
}
