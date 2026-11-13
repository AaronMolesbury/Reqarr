import type { MediaRequest, RequestStatus } from '../types/request'

export const statusLabels: Record<number, RequestStatus> = {
    1: 'Unknown', 2: 'Pending', 3: 'Processing', 4: 'Partially available', 5: 'Available', 6: 'Deleted',
}
export function statusOf(request: MediaRequest): RequestStatus { return (request.statusOverride as RequestStatus) || (statusLabels[request.mediaStatus] ?? 'Unknown') }
