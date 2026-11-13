export type IntegrationStatus = 'success' | 'partial' | 'error' | 'not_configured' | 'unknown'

export type Integrations = Record<'seerr' | 'radarr' | 'sonarr', IntegrationStatus>
