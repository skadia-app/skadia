import { useEffect, useState } from 'react'
import { getToken } from '@/lib/auth'

/**
 * Fetches an authenticated image URL and returns a blob URL for <img> rendering.
 * Needed because <img src="..."> cannot send Authorization headers.
 */
export function useAuthImage(url: string | undefined | null): string | null {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)

  useEffect(() => {
    if (!url) {
      setBlobUrl(null)
      return
    }

    let revoked = false
    const controller = new AbortController()

    const baseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'
    const fullUrl = url.startsWith('http') ? url : url.startsWith('/api/v1/')
      ? url.replace('/api/v1/', `${baseUrl}/`)
      : `${baseUrl}${url}`

    const token = getToken()
    const headers: Record<string, string> = {}
    if (token) headers['Authorization'] = `Bearer ${token}`

    fetch(fullUrl, { headers, credentials: 'include', signal: controller.signal })
      .then(res => {
        if (!res.ok) throw new Error(`${res.status}`)
        return res.blob()
      })
      .then(blob => {
        if (!revoked) setBlobUrl(URL.createObjectURL(blob))
      })
      .catch(() => {
        if (!revoked) setBlobUrl(null)
      })

    return () => {
      revoked = true
      controller.abort()
      setBlobUrl(prev => { if (prev) URL.revokeObjectURL(prev); return null })
    }
  }, [url])

  return blobUrl
}
