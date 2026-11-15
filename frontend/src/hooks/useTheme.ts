import * as React from "react"

type Theme = "light" | "dark"

export function useTheme() {
  const [theme, setTheme] = React.useState<Theme>(() =>
    document.documentElement.classList.contains("dark") ? "dark" : "light",
  )

  React.useEffect(() => {
    const root = document.documentElement
    root.classList.toggle("dark", theme === "dark")
    localStorage.setItem("catch-theme", theme)
  }, [theme])

  const toggle = React.useCallback(() => {
    setTheme((t) => (t === "dark" ? "light" : "dark"))
  }, [])

  return { theme, setTheme, toggle }
}
