import { build, preview } from 'vite'

const host = process.env.MODELPORT_VISUAL_HOST || '127.0.0.1'
const port = Number(process.env.MODELPORT_VISUAL_PORT || '4173')
const outDir = '.playwright-dist'

if (!Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error('MODELPORT_VISUAL_PORT must be an integer between 1 and 65535')
}

await build({
  build: {
    outDir,
    emptyOutDir: true,
  },
})

const server = await preview({
  build: { outDir },
  preview: {
    host,
    port,
    strictPort: true,
  },
})

let closing = false
async function closeServer() {
  if (closing) return
  closing = true
  await server.close()
}

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.once(signal, () => {
    void closeServer().finally(() => process.exit(0))
  })
}
