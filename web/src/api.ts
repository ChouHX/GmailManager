export interface Account {
  id: number
  email: string
  password: string
  totpSecret: string
  appPassword: string
  note: string
  hasPassword: boolean
  hasTotp: boolean
  hasAppPassword: boolean
  lastFetchAt: number
  lastFetchStatus: '' | 'ok' | 'error'
  lastFetchError: string
  lastFetchSubject: string
  lastFetchFrom: string
  lastFetchDate: number
  lastFetchSeen: boolean
  createdAt: number
  updatedAt: number
}

export interface AccountPage {
  items: Account[]
  total: number
  page: number
  pageSize: number
}

export interface Stats {
  total: number
  withAppPassword: number
  fetchOk: number
  fetchFailed: number
  fetchedToday: number
}

export interface AccountInput {
  email: string
  password: string
  totpSecret: string
  appPassword: string
  note: string
}

export interface MessageSummary {
  uid: number
  mailbox: string
  subject: string
  from: string
  fromName: string
  to: string
  date: number
  size: number
  seen: boolean
  flags: string[] | null
  snippet?: string
  messageId?: string
  hasAttachment: boolean
  folderLabel: string
}

export interface Attachment {
  filename: string
  mimeType: string
  size: number
  partPath: string
}

export interface MessageDetail extends MessageSummary {
  text: string
  html: string
  rawHtml: boolean
  attachments: Attachment[] | null
}

export interface FetchResult {
  accountId: number
  email: string
  ok: boolean
  error?: string
  messages: MessageSummary[] | null
  fetchedAt: number
  durationMs: number
}

export interface FetchLink {
  accountId: number
  email: string
  token: string
  url: string
  createdAt: number
  lastUsedAt: number
  useCount: number
}

export interface ImportResult {
  created: number
  updated: number
  skipped: number
  failed: number
  rows: { email: string; status: string; message: string }[]
}

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
    ...init,
  })

  const text = await res.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = text
    }
  }

  if (!res.ok) {
    const message =
      data && typeof data === 'object' && 'error' in (data as Record<string, unknown>)
        ? String((data as Record<string, unknown>).error)
        : `请求失败 (${res.status})`
    throw new ApiError(message, res.status)
  }
  return data as T
}

const json = (body: unknown): RequestInit => ({ method: 'POST', body: JSON.stringify(body) })

export const api = {
  login: (username: string, password: string) =>
    request<{ username: string; defaultPasswordInUse: boolean }>('/api/auth/login', json({ username, password })),
  logout: () => request<{ ok: boolean }>('/api/auth/logout', { method: 'POST' }),
  me: () => request<{ username: string; defaultPasswordInUse: boolean }>('/api/auth/me'),

  stats: () => request<Stats>('/api/stats'),

  listAccounts: (page: number, pageSize: number, q: string) =>
    request<AccountPage>(`/api/accounts?page=${page}&pageSize=${pageSize}&q=${encodeURIComponent(q)}`),
  createAccount: (input: AccountInput) => request<{ id: number }>('/api/accounts', json(input)),
  updateAccount: (id: number, input: AccountInput) =>
    request<{ ok: boolean }>(`/api/accounts/${id}`, { ...json(input), method: 'PUT' }),
  deleteAccount: (id: number) => request<{ ok: boolean }>(`/api/accounts/${id}`, { method: 'DELETE' }),
  deleteAccounts: (ids: number[]) => request<{ deleted: number }>('/api/accounts/batch-delete', json({ ids })),
  importAccounts: (content: string, overwrite: boolean) =>
    request<ImportResult>('/api/accounts/import', json({ content, overwrite })),
  exportUrl: (ids?: number[]) =>
    ids && ids.length ? `/api/accounts/export?ids=${ids.join(',')}` : '/api/accounts/export',
  verifyAccount: (id: number) =>
    request<{ ok: boolean; message: string }>(`/api/accounts/${id}/verify`, { method: 'POST' }),
  totp: (id: number) => request<{ code: string; remaining: number; period: number }>(`/api/accounts/${id}/totp`),

  accountLink: (id: number) => request<FetchLink>(`/api/accounts/${id}/link`),
  revokeLink: (id: number) =>
    request<{ ok: boolean }>(`/api/accounts/${id}/link/revoke`, { method: 'POST' }),
  bulkLinks: (ids: number[]) => request<{ links: FetchLink[] }>('/api/accounts/links', json({ ids })),
  exportLinksUrl: (ids?: number[]) =>
    ids && ids.length ? `/api/accounts/links/export?ids=${ids.join(',')}` : '/api/accounts/links/export',

  fetchAccount: (id: number, limit = 10, folder = 'auto') =>
    request<FetchResult>(`/api/fetch/${id}?limit=${limit}&folder=${folder}`),
  fetchBatch: (ids: number[], limit = 1, folder = 'auto') =>
    request<{ results: FetchResult[] }>('/api/fetch/batch', json({ ids, limit, folder })),
  message: (id: number, mailbox: string, uid: number) =>
    request<MessageDetail>(
      `/api/fetch/${id}/message?mailbox=${encodeURIComponent(mailbox)}&uid=${uid}`,
    ),
}
