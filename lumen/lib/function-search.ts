export const FUNCTION_SEARCH_EVENT = "nova:function-search"

export type FunctionSearchDetail = {
  query: string
}

export function readFunctionSearchFromLocation(): string {
  if (typeof window === "undefined") {
    return ""
  }
  const params = new URLSearchParams(window.location.search)
  return params.get("q") ?? ""
}

export function dispatchFunctionSearch(query: string) {
  if (typeof window === "undefined") {
    return
  }
  window.dispatchEvent(
    new CustomEvent<FunctionSearchDetail>(FUNCTION_SEARCH_EVENT, {
      detail: { query },
    })
  )
}
