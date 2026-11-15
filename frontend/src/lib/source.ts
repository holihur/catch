import type { Project } from "./types"

export interface SourceFrame {
  project: Project
  release?: string
  path: string
  line: number
  column?: number
  href?: string | null
}

export interface SourceLocation {
  file: string
  line: number
  column?: number
  start: number
  end: number
}

const EXTENSIONS = [
  "tsx", "ts", "jsx", "js", "mjs", "cjs", "vue", "svelte",
  "go", "py", "rb", "php", "java", "kt", "kts", "rs", "cs",
  "c", "cc", "cpp", "h", "hpp", "m", "mm", "swift", "dart",
  "scala", "ex", "exs", "erl", "clj", "sh", "sql",
].join("|")

const LOCATION_RE = new RegExp(
  `((?:(?:file|https?|webpack):\\/\\/)?[^\\s()\\[\\]]+?\\.(?:${EXTENSIONS})):(\\d+)(?::(\\d+))?`,
  "g",
)

const BUILT_ASSET_RE = /^(?:index|chunk|main|vendor|runtime|polyfills)[-.][A-Za-z0-9_-]{6,}\.(?:m?js|css)$/
const PYTHON_RE = /File\s+"([^"]+\.(?:py|pyw))",\s+line\s+(\d+)/i

function makeLocation(
  rawFile: string,
  rawLine: string | number,
  rawColumn: string | number | undefined,
  start: number,
  end: number,
): SourceLocation | null {
  const file = normalizePath(rawFile)
  if (!file) return null
  const base = file.split("/").pop() ?? ""
  if (
    file.includes("node_modules/") ||
    file.includes("/assets/") ||
    file.includes("/_next/") ||
    file.includes("/.next/") ||
    file.startsWith("assets/") ||
    BUILT_ASSET_RE.test(base) ||
    base.includes(".min.")
  ) {
    return null
  }
  // A bare basename cannot be mapped reliably without a source root.
  if (!file.includes("/")) return null

  const line = Number(rawLine)
  if (!Number.isFinite(line) || line <= 0) return null
  const column = rawColumn === undefined ? undefined : Number(rawColumn)
  return {
    file,
    line,
    column: Number.isFinite(column) ? column : undefined,
    start,
    end,
  }
}

/** Find the first file:line[:column] location in a stack line or culprit. */
export function parseSourceLocation(text: string): SourceLocation | null {
  LOCATION_RE.lastIndex = 0
  const match = LOCATION_RE.exec(text)
  if (match) {
    const loc = makeLocation(match[1], match[2], match[3], match.index, match.index + match[0].length)
    if (loc) return loc
  }
  const py = PYTHON_RE.exec(text)
  if (py) {
    return makeLocation(py[1], py[2], undefined, py.index, py.index + py[0].length)
  }
  return null
}

export function normalizePath(raw: string): string {
  let file = raw.trim()
  file = file.replace(/^(?:webpack|webpack-internal):\/\/+/, "")
  file = file.replace(/^file:\/\/+/, "")
  file = file.replace(/^https?:\/\/[^/]+/i, "")
  file = file.split(/[?#]/)[0]
  try {
    file = decodeURIComponent(file)
  } catch {
    // keep the raw path when it contains malformed escapes
  }
  file = file.replace(/\\/g, "/")
  file = file.replace(/^[A-Za-z]:\//, "")
  file = file.replace(/^\.?\//, "")
  file = file.replace(/^\/+/, "")
  file = file
    .split("/")
    .filter((segment) => segment && segment !== "." && segment !== "..")
    .join("/")
  return file
}

export function detectProvider(repoUrl: string): Project["repoProvider"] {
  try {
    const host = new URL(repoUrl).hostname.toLowerCase()
    if (host.includes("github")) return "github"
    if (host.includes("gitlab")) return "gitlab"
    if (host.includes("bitbucket")) return "bitbucket"
    if (host.includes("gitea")) return "gitea"
  } catch {
    // ignore
  }
  return "custom"
}

function cleanRepoUrl(repoUrl: string): string {
  let url = repoUrl.trim().replace(/\/+$/, "")
  url = url.replace(/\.git$/, "")
  return url
}

function encodePath(path: string): string {
  return path.split("/").map(encodeURIComponent).join("/")
}

function encodeBranch(branch: string): string {
  return branch.split("/").map(encodeURIComponent).join("/")
}

/** Resolve a stack path against the project's source root. */
export function resolveRepoPath(project: Project, file: string): string {
  let path = normalizePath(file)
  const root = (project.repoRoot ?? "").replace(/^\/+|\/+$/g, "")
  if (!root) return path

  const needle = `${root}/`
  const idx = path.indexOf(needle)
  if (idx >= 0) {
    path = path.slice(idx)
  }
  if (path === root) return path
  if (!path.startsWith(needle)) path = `${root}/${path}`
  return path.replace(/^\/+/, "")
}

/**
 * Build a deep link to the source repository. Returns null when the project has
 * no repository mapping or the location looks like a generated bundle.
 */
export function buildSourceUrl(
  project: Project | null | undefined,
  file: string,
  line: number,
  column?: number,
): string | null {
  if (!project) return null
  const path = resolveRepoPath(project, file)
  if (!path) return null
  const lineNo = Math.max(1, Math.floor(line))
  const branch = encodeBranch(project.repoBranch?.trim() || "main")
  const encodedPath = encodePath(path)
  const template = project.repoLinkTemplate?.trim()

  if (template && /^https?:\/\//i.test(template)) {
    return template
      .replaceAll("{path}", encodedPath)
      .replaceAll("{line}", String(lineNo))
      .replaceAll("{column}", String(column ?? 1))
      .replaceAll("{branch}", branch)
  }

  const repo = project.repoUrl ? cleanRepoUrl(project.repoUrl) : ""
  if (!repo) return null
  const provider = project.repoProvider || detectProvider(repo)

  switch (provider) {
    case "gitlab":
      return `${repo}/-/blob/${branch}/${encodedPath}#L${lineNo}`
    case "bitbucket":
      return `${repo}/src/${branch}/${encodedPath}#lines-${lineNo}`
    case "gitea":
      return `${repo}/src/branch/${branch}/${encodedPath}#L${lineNo}`
    case "custom":
      // A custom host needs an explicit URL template.
      return null
    case "github":
    default:
      return `${repo}/blob/${branch}/${encodedPath}#L${lineNo}`
  }
}

export function firstSourceFrame(
  project: Project | null | undefined,
  release: string | undefined,
  texts: Array<string | undefined | null>,
): SourceFrame | null {
  if (!project) return null
  for (const text of texts) {
    if (!text) continue
    for (const line of text.split("\n")) {
      const loc = parseSourceLocation(line)
      if (!loc) continue
      return {
        project,
        release,
        path: loc.file,
        line: loc.line,
        column: loc.column,
        href: buildSourceUrl(project, loc.file, loc.line, loc.column),
      }
    }
  }
  return null
}

export function buildSourceUrlForText(
  project: Project | null | undefined,
  text: string | undefined | null,
): string | null {
  if (!project || !text) return null
  const loc = parseSourceLocation(text)
  if (!loc) return null
  return buildSourceUrl(project, loc.file, loc.line, loc.column)
}

export function firstSourceUrl(
  project: Project | null | undefined,
  texts: Array<string | undefined | null>,
): string | null {
  for (const text of texts) {
    if (!text) continue
    for (const line of text.split("\n")) {
      const url = buildSourceUrlForText(project, line)
      if (url) return url
    }
  }
  return null
}

export function repoLabel(project: Project): string {
  if (project.repoUrl) {
    try {
      const url = new URL(project.repoUrl)
      return `${url.hostname.replace(/^www\./, "")}${url.pathname.replace(/\/+$/, "")}`
    } catch {
      return project.repoUrl
    }
  }
  return project.repoLinkTemplate ? "自定义链接模板" : ""
}
