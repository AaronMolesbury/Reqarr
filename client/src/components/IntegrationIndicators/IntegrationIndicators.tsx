import type { Integrations, IntegrationStatus } from '../../types/integration'
import './IntegrationIndicators.css'

type IntegrationIndicatorsProps = {
    integrations: Integrations | null
    stale: boolean
}

const labels: Record<IntegrationStatus, string> = {
    success: 'Latest checks succeeded',
    partial: 'Some data could not be retrieved',
    error: 'Latest check failed',
    not_configured: 'Not configured',
    unknown: 'Not checked',
}

const services = [
    { key: 'seerr', name: 'Seerr' },
    { key: 'radarr', name: 'Radarr' },
    { key: 'sonarr', name: 'Sonarr' },
] as const

export default function IntegrationIndicators({ integrations, stale }: IntegrationIndicatorsProps) {
    return (
        <div className="integrations" role="group" aria-label="Integration results from latest refresh">
            {services.map(({ key, name }) => {
                const status = integrations?.[key] ?? 'unknown'
                const appearance = stale && status !== 'not_configured' && status !== 'error' ? 'stale' : status
                const description = stale
                    ? `Dashboard refresh failed; results may be stale. ${labels[status]}`
                    : labels[status]

                return (
                    <span
                        key={key}
                        className="integration"
                        title={`${name}: ${description}`}
                        aria-label={`${name}: ${description}`}
                        tabIndex={0}
                    >
                        <span className={`integration-dot integration-dot-${appearance}`} aria-hidden="true" />
                        <span>{name}</span>
                        <span className="integration-tooltip" aria-hidden="true">
                            {description}
                        </span>
                    </span>
                )
            })}
        </div>
    )
}
