import { useEffect, useRef, useCallback } from 'react'
import { useQueryClient } from '@tanstack/react-query'

export function useJobStream() {
  const qc = useQueryClient()
  const esRef = useRef<EventSource | null>(null)

  const connect = useCallback(() => {
    if (esRef.current) return

    const es = new EventSource('/api/v1/jobs/stream')

    es.addEventListener('progress', (e) => {
      const data = JSON.parse((e as MessageEvent).data)
      // Jedes Progress-Event invalidiert die Jobs-Liste damit der Polling-Hook
      // auch aktuelle Daten bekommt wenn er gerade nicht aktiv pollt.
      qc.invalidateQueries({ queryKey: ['active-jobs'] })
      if (data.status === 'done') {
        qc.invalidateQueries({ queryKey: ['activity', data.activity_id] })
      }
    })

    es.addEventListener('init', (e) => {
      const jobs = JSON.parse((e as MessageEvent).data)
      qc.setQueryData(['active-jobs'], { jobs, pending_enrichments: 0 })
    })

    es.onerror = () => {
      es.close()
      esRef.current = null
      // Reconnect nach 5 Sekunden
      setTimeout(() => {
        if (esRef.current === null) connect()
      }, 5000)
    }

    esRef.current = es
  }, [qc])

  useEffect(() => {
    connect()
    return () => {
      esRef.current?.close()
      esRef.current = null
    }
  }, [connect])
}
