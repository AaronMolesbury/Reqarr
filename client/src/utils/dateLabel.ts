export function dateLabel(value: string) {
    if (!value || Number.isNaN(Date.parse(value))) return 'Unknown'
    return new Date(value).toLocaleString()
}
