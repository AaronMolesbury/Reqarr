import type { MediaRequest } from '../../types/request'
import { statusLabels, statusOf } from '../../utils/requestStatus'
import { dateLabel } from '../../utils/dateLabel'
import StatusPill from '../StatusPill/StatusPill'
import './RequestDetails.css'

type RequestDetailsProps = {
    request: MediaRequest
    onClose: () => void
}

export default function RequestDetails({ request, onClose }: RequestDetailsProps) {
    return (
        <div className="modal-backdrop" onClick={() => onClose()}>
            <section
                className="detail-modal"
                role="dialog"
                aria-modal="true"
                aria-labelledby="detail-title"
                onClick={(event) => event.stopPropagation()}
            >
                <div className="modal-heading">
                    <div>
                        <p className="eyebrow">
                            REQUEST #
                            {request.id}
                        </p>
                        <h2 id="detail-title">
                            {request.title}
                        </h2>
                    </div>
                    <button className="close-button" onClick={() => onClose()} aria-label="Close details">
                        ×
                    </button>
                </div>
                <div className="detail-line">
                    <span>Dashboard status</span>
                    <StatusPill status={statusOf(request)} reason={request.statusReason} />
                </div>
                {request.statusOverride && (
                    <div className="detail-line">
                        <span>Seerr status</span>
                        <strong>
                            {statusLabels[request.mediaStatus] ?? 'Unknown'}
                        </strong>
                    </div>
                )}
                {request.statusReason && (
                    <p className="modal-footnote">
                        {request.statusReason}
                    </p>
                )}
                {request.radarrStatus && (
                    <div className="detail-line">
                        <span>Radarr release status</span>
                        <strong>
                            {request.radarrStatus}
                        </strong>
                    </div>
                )}
                {request.sonarrStatus && (
                    <div className="detail-line">
                        <span>Sonarr series status</span>
                        <strong>
                            {request.sonarrStatus}
                        </strong>
                    </div>
                )}
                <div className="detail-line">
                    <span>Requester</span>
                    <strong>
                        {request.requester}
                    </strong>
                </div>
                <div className="detail-line">
                    <span>Request created</span>
                    <strong>
                        {dateLabel(request.createdAt)}
                    </strong>
                </div>
                <div className="detail-line">
                    <span>Last update</span>
                    <strong>
                        {dateLabel(request.updatedAt)}
                    </strong>
                </div>
                <div className="detail-note">
                    <strong>Latest information</strong>
                    <p>
                        {"Seerr reports "}
                        {(statusLabels[request.mediaStatus] ?? "Unknown").toLowerCase()}
                        .
                        {request.statusOverride ? ` ${request.type === "tv" ? "Sonarr" : "Radarr"} indicates ${request.statusOverride.toLowerCase()}.` : ""}
                        {" "}
                        {request.downloadEntries === 0 ? 'No download entries are present in its response.' : `${request.downloadEntries} download entries reported.`}
                    </p>
                </div>
                {request.statusOverride === 'Missing' && (
                    <p className="modal-footnote">
                        Radarr reports that this movie is eligible for download but has no imported file.
                        This does not mean a search or download failed.
                    </p>
                )}
                {request.activity?.length ? (
                    <div className="detail-note">
                        <strong>Recent activity (Radarr/Sonarr)</strong>
                        <ul className="activity-list">
                            {request.activity.map((event, index) => (
                                <li key={index}>
                                    <strong>
                                        {event.type}
                                    </strong>
                                    {" · "}
                                    {event.source}
                                    {event.date ? ` · ${dateLabel(event.date)}` : ''}
                                    {event.message ? ` — ${event.message}` : ''}
                                </li>
                            ))}
                        </ul>
                    </div>
                ) : (
                    <p className="modal-footnote">
                        No recent matching queue/history events returned. This does not prove a search failed.
                    </p>
                )}
                <p className="modal-footnote">
                    Activity is a recent upstream snapshot, not a permanent history. For TV, series-level activity may cover episodes outside the request.
                </p>
            </section>
        </div>
    )
}
