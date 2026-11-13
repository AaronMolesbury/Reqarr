import './RefreshControls.css'

type RefreshControlsProps = {
    loadedAt: Date | null
    loading: boolean
    autoRefreshIn: number | null
    retrying: boolean
    autoRefreshStopped: boolean
    onRefresh: () => Promise<void>
}

export default function RefreshControls({ loadedAt, loading, autoRefreshIn, retrying, autoRefreshStopped, onRefresh }: RefreshControlsProps) {
    return (
        <div className="refresh-controls">
            <span className="updated-label" aria-live="polite">
                {loadedAt ? `Last updated: ${loadedAt.toLocaleTimeString()}` : 'Not updated yet'}
            </span>
            <button
                type="button"
                className="refresh-button"
                onClick={() => void onRefresh()}
                disabled={loading}
                aria-label="Refresh dashboard data"
            >
                {loading ? 'Refreshing…' : 'Refresh'}
            </button>
            <span className="refresh-countdown" role="timer" aria-live="off">
                {loading
                    ? 'Updating dashboard'
                    : autoRefreshStopped
                        ? 'Auto refresh stopped after 3 failures'
                        : autoRefreshIn === null
                            ? 'Auto refresh paused'
                            : `${retrying ? 'Retry' : 'Auto refresh'} in ${autoRefreshIn}s`}
            </span>
        </div>
    )
}
