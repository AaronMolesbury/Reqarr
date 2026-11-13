import type { RequestStatus } from '../../types/request'
import './StatusPill.css'

export default function StatusPill({ status, reason }: { status: RequestStatus; reason?: string }) {
    return (
        <span
            className={`status status-${status.toLowerCase().replaceAll(' ', '-')}`}
            title={reason}
        >
            {status}
        </span>
    )
}
