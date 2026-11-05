import { useEffect, useMemo, useRef, useState } from 'react'
import { motion, useReducedMotion } from 'motion/react'
import {
  AlertCircle,
  ArrowRight,
  Eye,
  EyeOff,
  KeyRound,
  Loader2,
  ShieldCheck,
  User,
} from 'lucide-react'
import { useAuth } from '../components/AuthProvider'
import { ApiError } from '../api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'

const REMEMBER_KEY = 'gm-remember-user'

/** Persisted username, so repeat logins are one keystroke shorter. */
function readRemembered(): { user: string; remember: boolean } {
  try {
    const v = localStorage.getItem(REMEMBER_KEY)
    if (v) return { user: v, remember: true }
  } catch {
    /* storage blocked */
  }
  return { user: 'admin', remember: false }
}

export function LoginPage() {
  const { login } = useAuth()
  const remembered = useMemo(readRemembered, [])
  const [username, setUsername] = useState(remembered.user)
  const [password, setPassword] = useState('')
  const [remember, setRemember] = useState(remembered.remember)
  const [showPassword, setShowPassword] = useState(false)
  const [capsLock, setCapsLock] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [touched, setTouched] = useState(false)
  const passwordRef = useRef<HTMLInputElement>(null)
  const reduce = useReducedMotion()

  // Caps Lock is a classic source of "wrong password" reports.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (typeof e.getModifierState === 'function') {
        setCapsLock(e.getModifierState('CapsLock'))
      }
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('keyup', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('keyup', onKey)
    }
  }, [])

  const usernameError = touched && !username.trim() ? '请输入用户名' : ''
  const passwordError = touched && !password ? '请输入密码' : ''
  const canSubmit = Boolean(username.trim()) && Boolean(password) && !busy

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setTouched(true)
    if (busy || !username.trim() || !password) {
      if (!password) passwordRef.current?.focus()
      return
    }
    setError('')
    setBusy(true)
    try {
      await login(username.trim(), password)
      try {
        if (remember) localStorage.setItem(REMEMBER_KEY, username.trim())
        else localStorage.removeItem(REMEMBER_KEY)
      } catch {
        /* storage blocked */
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '登录失败，请稍后重试')
      setPassword('')
      passwordRef.current?.focus()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-full flex-col items-center justify-center bg-background px-4 py-10">
      <motion.div
        initial={reduce ? false : { opacity: 0, y: 8 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, ease: [0.23, 1, 0.32, 1] }}
        className="w-full max-w-[21.5rem]"
      >
        <div className="mb-6 flex flex-col items-center gap-2.5 text-center">
          <div className="grid size-10 place-items-center rounded-xl bg-primary text-[11.5px] font-bold text-primary-foreground shadow-sm">
            GM
          </div>
          <div>
            <p className="text-[14px] font-semibold tracking-tight">Gmail Manager</p>
            <p className="mt-0.5 text-[10.5px] text-muted-foreground">IMAP CONSOLE</p>
          </div>
        </div>

        <form
          onSubmit={submit}
          noValidate
          className="rounded-xl border border-border bg-card p-4 shadow-sm"
        >
          <h1 className="text-[13px] font-semibold">管理员登录</h1>
          <p className="mt-0.5 mb-4 text-[11px] text-muted-foreground">
            凭据仅保存在本容器内。
          </p>

          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="username" className="text-[11.5px] text-foreground/80">
                用户名
              </Label>
              <div className="relative">
                <User className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground/60" />
                <Input
                  id="username"
                  value={username}
                  onChange={(e) => {
                    setUsername(e.target.value)
                    setError('')
                  }}
                  onBlur={() => setTouched(true)}
                  autoComplete="username"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  autoFocus
                  disabled={busy}
                  aria-invalid={Boolean(usernameError)}
                  aria-describedby={usernameError ? 'username-error' : undefined}
                  className="h-9 pl-8"
                />
              </div>
              {usernameError && (
                <p id="username-error" className="text-[10.5px] text-destructive">
                  {usernameError}
                </p>
              )}
            </div>

            <div className="space-y-1.5">
              <div className="flex items-baseline justify-between">
                <Label htmlFor="password" className="text-[11.5px] text-foreground/80">
                  密码
                </Label>
                {capsLock && (
                  <span className="text-[10px] font-medium text-amber-600 dark:text-amber-400">
                    Caps Lock 已开启
                  </span>
                )}
              </div>
              <div className="relative">
                <KeyRound className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground/60" />
                <Input
                  ref={passwordRef}
                  id="password"
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value)
                    setError('')
                  }}
                  onBlur={() => setTouched(true)}
                  autoComplete="current-password"
                  disabled={busy}
                  aria-invalid={Boolean(passwordError)}
                  aria-describedby={passwordError ? 'password-error' : undefined}
                  className="h-9 pr-9 pl-8"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? '隐藏密码' : '显示密码'}
                  aria-pressed={showPassword}
                  tabIndex={-1}
                  className="absolute top-1/2 right-1.5 -translate-y-1/2 rounded p-1 text-muted-foreground/60 transition-colors hover:text-foreground"
                >
                  {showPassword ? <EyeOff className="size-3.5" /> : <Eye className="size-3.5" />}
                </button>
              </div>
              {passwordError && (
                <p id="password-error" className="text-[10.5px] text-destructive">
                  {passwordError}
                </p>
              )}
            </div>

            <div className="flex items-center gap-2 pt-0.5">
              <Checkbox
                id="remember"
                checked={remember}
                onCheckedChange={(v) => setRemember(v === true)}
                disabled={busy}
              />
              <Label
                htmlFor="remember"
                className="cursor-pointer text-[11.5px] font-normal text-muted-foreground"
              >
                记住用户名
              </Label>
            </div>

            {error && (
              <motion.div
                role="alert"
                initial={reduce ? false : { opacity: 0, y: -4 }}
                animate={{ opacity: 1, y: 0 }}
                className="flex items-start gap-1.5 rounded-md border border-destructive/25 bg-destructive/8 px-2.5 py-2 text-[11.5px] text-destructive"
              >
                <AlertCircle className="mt-px size-3.5 shrink-0" />
                <span className="leading-relaxed">{error}</span>
              </motion.div>
            )}

            <Button type="submit" disabled={!canSubmit} className="h-9 w-full gap-1.5 text-[13px]">
              {busy ? (
                <>
                  <Loader2 className="size-3.5 animate-spin" />
                  登录中…
                </>
              ) : (
                <>
                  登录
                  <ArrowRight className="size-3.5" />
                </>
              )}
            </Button>
          </div>
        </form>

        <p className="mt-4 flex items-start justify-center gap-1.5 px-1 text-center text-[10px] leading-relaxed text-muted-foreground/80">
          <ShieldCheck className="mt-px size-2.5 shrink-0" />
          <span>
            首次部署请用 <code className="font-mono">GM_ADMIN_PASSWORD</code> 修改默认密码
          </span>
        </p>
      </motion.div>
    </div>
  )
}
