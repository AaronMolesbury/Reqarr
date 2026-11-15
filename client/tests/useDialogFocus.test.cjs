const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const test = require('node:test')
const vm = require('node:vm')
const ts = require('typescript')

// Controlled DOM focus verifies the hook's keyboard lifecycle without a browser.
function harness() {
    const listeners = new Map()
    const effects = []
    const refs = []
    const document = {
        activeElement: null,
        addEventListener: (type, handler) => listeners.set(type, handler),
        removeEventListener: (type) => listeners.delete(type),
    }
    const element = () => ({
        isConnected: true,
        focus() { document.activeElement = this },
    })
    const trigger = element()
    trigger.focus()
    const first = element()
    const last = element()
    const dialog = element()
    let elements = [first, last]
    dialog.querySelectorAll = () => elements
    dialog.contains = (target) => target === dialog || elements.includes(target)
    const react = {
        useRef(value) {
            const ref = { current: value }
            refs.push(ref)
            return ref
        },
        useEffect(effect) { effects.push(effect) },
    }
    const source = fs.readFileSync(path.join(__dirname, '../src/hooks/useDialogFocus.ts'), 'utf8')
    const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText
    const exports = {}
    vm.runInNewContext(compiled, { exports, require: () => react, document })
    let closed = 0
    const ref = exports.useDialogFocus(() => closed++)
    ref.current = dialog
    effects[0]()
    const cleanup = effects[1]()
    const key = (name, shiftKey = false) => {
        const event = { key: name, shiftKey, prevented: false, stopped: false, preventDefault() { this.prevented = true }, stopPropagation() { this.stopped = true } }
        listeners.get('keydown')(event)
        return event
    }
    return { document, listeners, refs, trigger, first, last, dialog, cleanup, key, closed: () => closed, empty: () => { elements = [] } }
}

test('dialog receives focus, contains keyboard navigation and restores the trigger', () => {
    const h = harness()
    assert.equal(h.document.activeElement, h.first)
    assert.equal(h.key('Tab', true).prevented, true)
    assert.equal(h.document.activeElement, h.last)
    assert.equal(h.key('Tab').prevented, true)
    assert.equal(h.document.activeElement, h.first)
    assert.equal(h.key('Tab').prevented, false)
    h.trigger.focus()
    h.listeners.get('focusin')()
    assert.equal(h.document.activeElement, h.first)
    h.cleanup()
    assert.equal(h.document.activeElement, h.trigger)
    assert.equal(h.listeners.size, 0)
})

test('Escape closes using the current callback and handles an empty dialog', () => {
    const h = harness()
    let latestClosed = false
    h.refs[1].current = () => { latestClosed = true }
    const event = h.key('Escape')
    assert.equal(event.prevented, true)
    assert.equal(event.stopped, true)
    assert.equal(latestClosed, true)
    assert.equal(h.closed(), 0)
    h.empty()
    h.dialog.focus()
    assert.equal(h.key('Tab').prevented, true)
    assert.equal(h.document.activeElement, h.dialog)
    h.trigger.isConnected = false
    h.cleanup()
    assert.equal(h.document.activeElement, h.dialog)
})
