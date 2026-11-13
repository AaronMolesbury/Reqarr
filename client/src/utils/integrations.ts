import type { Integrations, IntegrationStatus } from '../types/integration'

export function parseIntegrations(value: unknown): Integrations | null {
    if (!value || typeof value !== 'object') return null

    const statuses: IntegrationStatus[] = ['success', 'partial', 'error', 'not_configured', 'unknown']
    const record = value as Record<string, unknown>
    if (!['seerr', 'radarr', 'sonarr'].every((key) => statuses.includes(record[key] as IntegrationStatus))) {
        return null
    }

    return value as Integrations
}
