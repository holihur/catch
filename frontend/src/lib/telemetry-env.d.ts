/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_URL?: string
  readonly VITE_INGEST_KEY?: string
  readonly VITE_INGEST_ENDPOINT?: string
  readonly VITE_ENV?: string
  readonly VITE_RELEASE?: string
  readonly VITE_APP_NAME?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
