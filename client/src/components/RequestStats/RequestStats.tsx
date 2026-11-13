import type { MediaRequest } from '../../types/request'
import { statusOf } from '../../utils/requestStatus'
import './RequestStats.css'

export default function RequestStats({ requests }: { requests: MediaRequest[] }) {
    const availableCount = requests.filter((r) => statusOf(r) === 'Available').length
    const partialCount = requests.filter((r) => statusOf(r) === 'Partially available').length
    const missingCount = requests.filter((r) => statusOf(r) === 'Missing').length
    const processingCount = requests.filter((r) => ['Processing', 'Downloading', 'Awaiting import', 'Queued'].includes(statusOf(r))).length
    const awaitingCount = requests.filter((r) => statusOf(r) === 'Awaiting release').length

    return (
        <section className="stats" aria-label="Request statistics">
            <article className="stat-card">
                <span className="stat-label">Total requests</span>
                <strong>
                    {requests.length}
                </strong>
                <span className="stat-note">
                    {missingCount ? `From Seerr · ${missingCount} missing` : 'From Seerr'}
                </span>
            </article>
            <article className="stat-card">
                <span className="stat-label">Processing</span>
                <strong>
                    {processingCount}
                </strong>
                <span className="stat-note">
                    {awaitingCount ? `${awaitingCount} awaiting release` : "Download stage not yet known"}
                </span>
            </article>
            <article className="stat-card">
                <span className="stat-label">Available</span>
                <strong>
                    {availableCount}
                </strong>
                <span className="stat-note">Reported by Seerr</span>
            </article>
            <article className="stat-card">
                <span className="stat-label">Partially available</span>
                <strong>
                    {partialCount}
                </strong>
                <span className="stat-note">Not necessarily a failure</span>
            </article>
        </section>
    )
}
