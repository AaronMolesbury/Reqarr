import type { Integrations } from './integration'

export type RequestStatus = 'Available' | 'Partially available' | 'Processing' | 'Missing' | 'Awaiting release' | 'Downloading' | 'Awaiting import' | 'Queued' | 'Needs attention' | 'Pending' | 'Unknown' | 'Deleted'

export type MediaRequest = {
    id: number
    title: string
    type: 'movie' | 'tv'
    requester: string
    requestStatus: number
    mediaStatus: number
    updatedAt: string
    createdAt: string
    tmdbId: number
    tvdbId?: number
    downloadEntries: number
    statusOverride?: string
    statusReason?: string
    radarrStatus?: string
    sonarrStatus?: string
    activity?: { source: string; type: string; date?: string; message?: string }[]
}

export type DashboardResponse = {
    checkedAt?: string
    requests: MediaRequest[]
    total: number
    integrations: Integrations
}
