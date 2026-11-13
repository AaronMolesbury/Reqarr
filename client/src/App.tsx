import { useState } from 'react'
import Header from './components/Header/Header'
import RefreshControls from './components/RefreshControls/RefreshControls'
import RefreshError from './components/RefreshError/RefreshError'
import RequestStats from './components/RequestStats/RequestStats'
import RequestTable from './components/RequestTable/RequestTable'
import RequestDetails from './components/RequestDetails/RequestDetails'
import { useDashboard } from './hooks/useDashboard'
import type { RequestStatus } from './types/request'

export default function App() {
    const { requests, loading, error, loadedAt, refresh, integrations, autoRefreshIn, autoRefreshStopped } = useDashboard()
    const [filter, setFilter] = useState<'All' | RequestStatus>('All')
    const [selectedId, setSelectedId] = useState<number | null>(null)
    const selected = requests.find((request) => request.id === selectedId)

    return (
        <main className="app-shell">
            <Header integrations={integrations} stale={Boolean(error)} />
            <section className="page">
                <div className="page-heading">
                    <div>
                        <p className="eyebrow">OVERVIEW</p>
                        <h1>Dashboard</h1>
                        <p className="muted">
                            A quick view of Seerr requests, enriched with Radarr and Sonarr release information when connected.
                        </p>
                    </div>
                    <RefreshControls
                        loadedAt={loadedAt}
                        loading={loading}
                        autoRefreshIn={autoRefreshIn}
                        retrying={Boolean(error)}
                        autoRefreshStopped={autoRefreshStopped}
                        onRefresh={refresh}
                    />
                </div>
                {error && (
                    <RefreshError error={error} />
                )}
                <RequestStats requests={requests} />
                <RequestTable
                    requests={requests}
                    filter={filter}
                    onFilterChange={setFilter}
                    onSelect={(request) => setSelectedId(request.id)}
                    loading={loading}
                    error={error}
                />
                <p className="footer-note">
                    Availability starts with Seerr. Radarr can identify movies awaiting release; Sonarr can identify upcoming series. Queue states and recent history come from Radarr/Sonarr. History is not a complete event timeline and does not prove current availability.
                </p>
            </section>
            {selected && (
                <RequestDetails request={selected} onClose={() => setSelectedId(null)} />
            )}
        </main>
    )
}
