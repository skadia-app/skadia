# Toast Upload Progress Example

## Use Case

Bei der Verarbeitung von mehreren Dateien-Uploads (z.B. 200 Garmin FIT-Dateien):

- **ALT (Ohne Grouping):** 200 einzelne Toast-Nachrichten → User muss 200x wegklicken oder warten bis sie auto-dismiss
- **NEU (Mit Grouping):** 1 Update-Toast mit Batch-Progress + separate Error-Toast → "120/200 hochgeladen"

## Implementation

### 1. Start Upload Batch

```tsx
import { useToast } from '@/lib/toast'

const { addToast, clearGroup } = useToast()

// Create single progress toast that won't auto-dismiss
const uploadBatchToastId = addToast({
  type: 'info',
  title: '0/200 hochgeladen',
  duration: 0,  // Don't auto-dismiss during batch
  group: 'upload-batch'
})
```

### 2. Update Progress (Remove & Re-add Pattern)

Da das Toast-System kein `updateToast()` hat, verwende das **Remove & Re-add Pattern**:

```tsx
const { addToast, removeToast, clearGroup } = useToast()

let currentToastId = uploadBatchToastId

async function uploadBatch(files: File[]) {
  const errors: Array<{ filename: string; error: string }> = []

  for (let i = 0; i < files.length; i++) {
    try {
      await uploadFile(files[i])

      // Remove old progress toast and create new one with updated progress
      removeToast(currentToastId)
      currentToastId = addToast({
        type: 'info',
        title: `${i + 1}/${files.length} hochgeladen`,
        duration: 0,
        group: 'upload-batch'
      })
    } catch (err) {
      errors.push({
        filename: files[i].name,
        error: err instanceof Error ? err.message : 'Unbekannter Fehler'
      })
    }
  }

  // Remove progress toast
  removeToast(currentToastId)

  // Show completion or errors
  handleUploadCompletion(files.length, errors)
}
```

### 3. Handle Errors (Separate Toast)

```tsx
function handleUploadCompletion(total: number, errors: Array<{ filename: string; error: string }>) {
  if (errors.length === 0) {
    // Success: single completion toast
    addToast({
      type: 'success',
      title: `${total} Dateien erfolgreich hochgeladen`,
      group: 'upload-batch'
    })
  } else {
    // Show summary toast
    addToast({
      type: 'error',
      title: `${errors.length}/${total} Uploads fehlgeschlagen`,
      message: errors
        .slice(0, 5)  // Show first 5 errors
        .map(e => `${e.filename}: ${e.error}`)
        .join('\n') + (errors.length > 5 ? `\n... und ${errors.length - 5} weitere` : ''),
      duration: 0,
      group: 'upload-batch'
    })
  }
}
```

### 4. Batch-Ende: Clear All Grouped Toasts

```tsx
// When user dismisses error or needs fresh start:
clearGroup('upload-batch')
```

## Alternative: Progress State Pattern

Wenn häufige Updates das DOM belasten, nutze ein **Debounce Pattern**:

```tsx
import { useRef } from 'react'

function uploadBatchWithDebounce(files: File[]) {
  const { addToast, removeToast, clearGroup } = useToast()
  const progressToastIdRef = useRef<string | null>(null)
  const updateTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  let uploaded = 0
  const total = files.length

  // Initial toast
  progressToastIdRef.current = addToast({
    type: 'info',
    title: '0/' + total + ' hochgeladen',
    duration: 0,
    group: 'upload-batch'
  })

  for (const file of files) {
    uploadFile(file)
      .then(() => {
        uploaded++

        // Debounced update: only show every 500ms
        clearTimeout(updateTimeoutRef.current!)
        updateTimeoutRef.current = setTimeout(() => {
          if (progressToastIdRef.current) {
            removeToast(progressToastIdRef.current)
            progressToastIdRef.current = addToast({
              type: 'info',
              title: `${uploaded}/${total} hochgeladen`,
              duration: 0,
              group: 'upload-batch'
            })
          }
        }, 500)
      })
      .catch(err => {
        // Handle individual errors without clearing progress
        console.error(`Upload failed: ${file.name}`, err)
      })
  }

  // Note: Real implementation would need Promise.all or Promise.allSettled
}
```

## Benefits

✅ **Single Source of Truth:** 1 Toast zeigt Progress, nicht 200
✅ **Clean Separation:** Progress-Toast + Error-Toast in gleicher `group`
✅ **Batch Control:** `clearGroup()` löscht alle zusammenhängenden Toasts
✅ **No Auto-Dismiss:** `duration: 0` verhindert Ablenkung während Upload läuft
✅ **Scaling:** Pattern funktioniert für 5 Dateien wie für 500

## Related API

```ts
interface ToastContextType {
  toasts: Toast[]

  // Create new toast, returns ID (string)
  addToast: (toast: Omit<Toast, 'id'>) => string

  // Remove single toast by ID
  removeToast: (id: string) => void

  // Remove all toasts with matching group
  clearGroup: (group: string) => void
}

interface Toast {
  id: string
  type: 'success' | 'error' | 'warning' | 'info'
  title: string
  message?: string
  duration?: number  // milliseconds; 0 = no auto-dismiss
  group?: string     // optional grouping key
}
```
