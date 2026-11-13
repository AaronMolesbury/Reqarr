import './Header.css'
import IntegrationIndicators from '../IntegrationIndicators/IntegrationIndicators'
import type { Integrations } from '../../types/integration'

type HeaderProps = {
    integrations: Integrations | null
    stale: boolean
}

export default function Header({ integrations, stale }: HeaderProps) {
    return (
        <header className="topbar">
            <div className="brand">
                <div className="brand-mark">R</div>
                <div>
                    <div className="brand-name">Reqarr</div>
                    <div className="brand-subtitle">Request activity</div>
                </div>
            </div>
            <IntegrationIndicators integrations={integrations} stale={stale} />
        </header>
    )
}
