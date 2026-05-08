// Vitest setup. Provides a basic localStorage stub for jsdom (older jsdom
// versions used in this project don't expose it on window).

class MemoryStorage {
  private store = new Map<string, string>()
  get length() { return this.store.size }
  clear() { this.store.clear() }
  getItem(key: string) { return this.store.has(key) ? this.store.get(key)! : null }
  setItem(key: string, value: string) { this.store.set(key, String(value)) }
  removeItem(key: string) { this.store.delete(key) }
  key(i: number) { return Array.from(this.store.keys())[i] ?? null }
}

const storage = new MemoryStorage()
const target: any = typeof window !== 'undefined' ? window : globalThis
try {
  Object.defineProperty(target, 'localStorage', {
    value: storage,
    configurable: true,
    writable: true,
  })
} catch {
  target.localStorage = storage
}
if (typeof window !== 'undefined' && typeof globalThis !== 'undefined') {
  ;(globalThis as any).localStorage = storage
}
