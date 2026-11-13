import './RefreshError.css'

export default function RefreshError({ error }: { error: string }) {
    return (
        <section className="attention-banner" role="alert">
            <div className="attention-icon">!</div>
            <div className="attention-copy">
                <strong>Could not refresh dashboard</strong>
                <span>
                    {error}
                    . Previous data remains visible if available.
                </span>
            </div>
        </section>
    )
}
