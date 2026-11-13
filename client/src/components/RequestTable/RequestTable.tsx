import { useMemo } from 'react'
import type { MediaRequest, RequestStatus } from '../../types/request'
import { statusOf } from '../../utils/requestStatus'
import { dateLabel } from '../../utils/dateLabel'
import StatusPill from '../StatusPill/StatusPill'
import './RequestTable.css'

type RequestTableProps = {
    requests: MediaRequest[]
    filter: 'All' | RequestStatus
    onFilterChange: (filter: 'All' | RequestStatus) => void
    onSelect: (request: MediaRequest) => void
    loading: boolean
    error: string
}

export default function RequestTable({ requests, filter, onFilterChange, onSelect, loading, error }: RequestTableProps) {
    const visibleRequests = useMemo(() => filter === 'All' ? requests : requests.filter((r) => statusOf(r) === filter), [filter, requests])

    return (
        <section className="requests-panel">
            <div className="panel-heading">
                <div>
                    <h2>Recent requests</h2>
                    <p className="muted">Seerr availability with optional Radarr/Sonarr status</p>
                </div>
                <label className="filter-control">
                    <span className="sr-only">Filter requests</span>
                    <select
                        value={filter}
                        onChange={(event) => onFilterChange(event.target.value as 'All' | RequestStatus)}
                    >
                        <option value="All">All requests</option>
                        <option value="Processing">Processing</option>
                        <option value="Missing">Missing</option>
                        <option value="Awaiting release">Awaiting release</option>
                        <option value="Downloading">Downloading</option>
                        <option value="Awaiting import">Awaiting import</option>
                        <option value="Queued">Queued</option>
                        <option value="Needs attention">Needs attention</option>
                        <option value="Partially available">Partially available</option>
                        <option value="Available">Available</option>
                        <option value="Pending">Pending</option>
                        <option value="Unknown">Unknown</option>
                        <option value="Deleted">Deleted</option>
                    </select>
                </label>
            </div>
            <div className="table-wrap">
                <table>
                    <thead>
                        <tr>
                            <th>Title</th>
                            <th>Requester</th>
                            <th>Status</th>
                            <th>Last updated</th>
                            <th aria-label="Details" />
                        </tr>
                    </thead>
                    <tbody>
                        {visibleRequests.map((request) => (
                            <tr
                                key={request.id}
                                className="request-row"
                                onClick={() => onSelect(request)}
                                tabIndex={0}
                                onKeyDown={(event) => {
                                    if (event.key === 'Enter' || event.key === ' ') {
                                        event.preventDefault()
                                        onSelect(request)
                                    }
                                }}
                            >
                                <td>
                                    <span className="request-title">
                                        {request.title}
                                    </span>
                                    <span className="request-meta">
                                        {request.type === 'tv' ? 'TV' : 'Movie'}
                                        {" · #"}
                                        {request.id}
                                    </span>
                                </td>
                                <td>
                                    {request.requester}
                                </td>
                                <td>
                                    <StatusPill status={statusOf(request)} reason={request.statusReason} />
                                </td>
                                <td className="muted">
                                    {dateLabel(request.updatedAt)}
                                </td>
                                <td className="row-arrow">→</td>
                            </tr>
                        ))}
                        {visibleRequests.length === 0 && !loading && (
                            <tr>
                                <td colSpan={5} className="empty-state">
                                    {error ? 'No data loaded.' : 'No requests match this filter.'}
                                </td>
                            </tr>
                        )}
                        {loading && requests.length === 0 && (
                            <tr>
                                <td colSpan={5} className="empty-state">Loading requests…</td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>
        </section>
    )
}
