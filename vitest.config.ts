import { configDefaults, defineConfig } from 'vitest/config'
import { resolve } from 'path'

export default defineConfig({
  test: {
    exclude: [...configDefaults.exclude, 'dist', 'vendor'],
    alias: {
      "@go/*": resolve(__dirname, "./vendor/*"),
    },
  },
})
