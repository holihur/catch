import * as React from "react"

import { api } from "@/lib/api"
import type { Project } from "@/lib/types"
import { useAuth } from "./AuthContext"

interface ProjectContextValue {
  projects: Project[]
  loading: boolean
  selectedId: string
  selected: Project | null
  setSelectedId: (id: string) => void
  refreshProjects: () => Promise<void>
}

const ProjectContext = React.createContext<ProjectContextValue | null>(null)
const STORAGE_KEY = "catch:project"

export function ProjectProvider({ children }: { children: React.ReactNode }) {
  const { me } = useAuth()
  const [projects, setProjects] = React.useState<Project[]>([])
  const [loading, setLoading] = React.useState(true)
  const [selectedId, setSelectedIdState] = React.useState(
    () => localStorage.getItem(STORAGE_KEY) ?? "",
  )

  const refreshProjects = React.useCallback(async () => {
    if (!me) return
    setLoading(true)
    try {
      const res = await api.projects()
      setProjects(res.projects)
      setSelectedIdState((prev) => {
        if (prev && !res.projects.some((p) => p.id === prev)) {
          localStorage.removeItem(STORAGE_KEY)
          return ""
        }
        return prev
      })
    } catch (err) {
      console.warn("load projects failed", err)
    } finally {
      setLoading(false)
    }
  }, [me])

  React.useEffect(() => {
    if (me) void refreshProjects()
    else {
      setProjects([])
      setLoading(false)
    }
  }, [me, refreshProjects])

  const setSelectedId = React.useCallback((id: string) => {
    setSelectedIdState(id)
    if (id) localStorage.setItem(STORAGE_KEY, id)
    else localStorage.removeItem(STORAGE_KEY)
  }, [])

  const selected = projects.find((p) => p.id === selectedId) ?? null

  const value = React.useMemo(
    () => ({ projects, loading, selectedId, selected, setSelectedId, refreshProjects }),
    [projects, loading, selectedId, selected, setSelectedId, refreshProjects],
  )
  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>
}

export function useProjects() {
  const ctx = React.useContext(ProjectContext)
  if (!ctx) throw new Error("useProjects must be used within ProjectProvider")
  return ctx
}
