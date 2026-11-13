const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const test = require('node:test')
const vm = require('node:vm')
const ts = require('typescript')

// Exercise the real hook with controlled effects, clock, visibility and fetch.
// This harness does not run a browser or contact the API.
function harness(fetchResponse) {
    let now = 1_000_000
    let cursor = 0
    let effect
    let cleanup
    const slots = []
    const timers = new Map()
    const listeners = new Map()
    const calls = []
    let timerID = 0
    const document = {
        visibilityState: 'visible',
        addEventListener: (name, callback) => listeners.set(name, callback),
        removeEventListener: (name) => listeners.delete(name),
    }
    const react = {
        useState(initial) {
            const index = cursor++
            if (!(index in slots)) slots[index] = initial
            return [slots[index], (value) => {
                slots[index] = typeof value === 'function' ? value(slots[index]) : value
            }]
        },
        useRef(initial) {
            const index = cursor++
            return slots[index] ??= { current: initial }
        },
        useCallback(callback) {
            const index = cursor++
            return slots[index] ??= callback
        },
        useEffect(callback) {
            cursor++
            effect = callback
        },
    }
    function load(relativePath) {
        const file = path.join(__dirname, '../src', relativePath)
        const code = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
            compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
        }).outputText
        const exports = {}
        vm.runInNewContext(code, {
            exports,
            require(name) {
                if (name === 'react') return react
                if (name === '../utils/integrations') return load('utils/integrations.ts')
                throw new Error(`Unexpected import: ${name}`)
            },
            Date: class extends Date { static now() { return now } },
            Error,
            AbortController,
            document,
            window: {
                setInterval(callback, interval) {
                    const id = ++timerID
                    timers.set(id, { callback, interval, next: now + interval })
                    return id
                },
                clearInterval: (id) => timers.delete(id),
            },
            fetch: (url, options) => {
                calls.push({ url, signal: options.signal })
                return fetchResponse(calls.length, options.signal)
            },
        }, { filename: file })
        return exports
    }
    const { useDashboard } = load('hooks/useDashboard.ts')
    const render = () => { cursor = 0; return useDashboard() }
    const flush = async () => { for (let index = 0; index < 10; index++) await Promise.resolve() }
    render()
    cleanup = effect()
    return {
        calls,
        render,
        flush,
        async advance(ms) {
            const end = now + ms
            while (true) {
                const next = Math.min(...[...timers.values()].map((timer) => timer.next))
                if (next > end) break
                now = next
                for (const timer of timers.values()) {
                    if (timer.next <= now) {
                        timer.next += timer.interval
                        timer.callback()
                    }
                }
                await flush()
            }
            now = end
        },
        visible(value) {
            document.visibilityState = value ? 'visible' : 'hidden'
            listeners.get('visibilitychange')?.()
        },
        replayEffect() { cleanup(); cleanup = effect() },
        stop() { cleanup() },
    }
}

function response(status, requests = []) {
    return Promise.resolve({
        ok: status === 200,
        status,
        json: async () => ({ requests, integrations: { seerr: 'success', radarr: 'not_configured', sonarr: 'not_configured' } }),
    })
}

test('502 retry recovers after 5 seconds, then returns to 90 seconds', async () => {
    const h = harness((call) => call === 1
        ? Promise.resolve({ ok: false, status: 502, json: async () => { throw new Error('not JSON') } })
        : response(200, [{ id: 1 }]))
    await h.flush()
    assert.match(h.render().error, /HTTP 502/)
    assert.equal(h.render().autoRefreshIn, 5)
    await h.advance(4000)
    assert.equal(h.calls.length, 1)
    await h.advance(1000)
    assert.equal(h.calls.length, 2)
    assert.equal(h.render().error, '')
    assert.equal(h.render().autoRefreshIn, 90)
    await h.advance(89_000)
    assert.equal(h.calls.length, 2)
    await h.advance(1000)
    assert.equal(h.calls.length, 3)
    h.stop()
})

test('three consecutive failures stop polling, preserve data, and allow manual recovery', async () => {
    const requests = [{ id: 1 }]
    let recovered = false
    const h = harness((call) => response(call === 1 || recovered ? 200 : 502, requests))
    await h.flush()
    const loadedAt = h.render().loadedAt
    await h.render().refresh()
    for (const delay of [5, 10]) {
        assert.equal(h.render().autoRefreshIn, delay)
        assert.equal(h.render().requests, requests)
        assert.equal(h.render().loadedAt, loadedAt)
        await h.advance(delay * 1000)
    }
    assert.equal(h.calls.length, 4) // Initial success, then exactly three failures.
    assert.equal(h.render().autoRefreshStopped, true)
    assert.equal(h.render().autoRefreshIn, null)
    await h.advance(180_000)
    h.visible(false)
    h.visible(true)
    await h.flush()
    assert.equal(h.calls.length, 4)
    recovered = true
    await h.render().refresh()
    assert.equal(h.calls.length, 5)
    assert.equal(h.render().autoRefreshStopped, false)
    assert.equal(h.render().autoRefreshIn, 90)
    assert.equal(h.render().error, '')
    await h.advance(90_000)
    assert.equal(h.calls.length, 6)
    h.stop()
})

test('hidden tabs pause retries and immediately retry when overdue on return', async () => {
    const h = harness(() => response(502))
    await h.flush()
    h.visible(false)
    await h.advance(60_000)
    assert.equal(h.calls.length, 1)
    assert.equal(h.render().autoRefreshIn, null)
    h.visible(true)
    await h.flush()
    assert.equal(h.calls.length, 2)
    h.stop()
})

test('StrictMode cancellation allows an immediate replacement without unlocking it', async () => {
    const pending = []
    const h = harness(() => new Promise((resolve) => pending.push(resolve)))
    h.replayEffect()
    assert.equal(h.calls.length, 2)
    assert.equal(h.calls[0].signal.aborted, true)
    pending[0](await response(200, [{ id: 99 }]))
    await h.flush()
    await h.render().refresh()
    await h.advance(5000)
    assert.equal(h.calls.length, 2)
    assert.equal(h.render().loading, true)
    assert.equal(h.render().requests.length, 0)
    pending[1](await response(200, [{ id: 1 }]))
    await h.flush()
    assert.equal(h.render().requests[0].id, 1)
    assert.equal(h.render().loading, false)
    h.stop()
})
