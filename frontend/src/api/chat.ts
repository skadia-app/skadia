import { apiClient } from '@/api/client'

export interface ChatSession {
  id: string
  title: string | null
  created_at: string
  updated_at: string
}

export interface ChatMessage {
  id: string
  session_id: string
  role: 'user' | 'assistant' | 'tool'
  content: string
  tool_calls?: unknown
  sources?: string[]
  created_at: string
}

export function listSessions(): Promise<ChatSession[]> {
  return apiClient<ChatSession[]>('/chat/sessions')
}

export function createSession(): Promise<ChatSession> {
  return apiClient<ChatSession>('/chat/sessions', { method: 'POST' })
}

export function getMessages(sessionId: string): Promise<ChatMessage[]> {
  return apiClient<ChatMessage[]>(`/chat/sessions/${sessionId}/messages`)
}

export function deleteSession(sessionId: string): Promise<void> {
  return apiClient<void>(`/chat/sessions/${sessionId}`, { method: 'DELETE' })
}
