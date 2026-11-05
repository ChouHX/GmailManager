import { useEffect, useMemo, useState } from 'react'
import { Copy, Eye, EyeOff, KeyRound, Loader2, ShieldCheck } from 'lucide-react'
import { api, ApiError, type Account, type AccountInput } from '../api'
import { useToast } from '../components/ToastProvider'
import { copyText } from '../utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

const EMPTY: AccountInput = { email: '', password: '', totpSecret: '', appPassword: '', note: '' }
const MASK = '••••••••'

interface AccountDialogProps {
  open: boolean
  account: Account | null
  onClose: () => void
  onSaved: () => void
}

export function AccountDialog({ open, account, onClose, onSaved }: AccountDialogProps) {
  const { toast } = useToast()
  const [form, setForm] = useState<AccountInput>(EMPTY)
  const [show, setShow] = useState({ password: false, totp: false, app: false })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [verify, setVerify] = useState<{ state: 'idle' | 'busy' | 'ok' | 'fail'; message: string }>({
    state: 'idle',
    message: '',
  })

  const editing = Boolean(account)

  useEffect(() => {
    if (!open) return
    setError('')
    setVerify({ state: 'idle', message: '' })
    setShow({ password: false, totp: false, app: false })
    if (account) {
      setForm({
        email: account.email,
        password: account.hasPassword ? MASK : '',
        totpSecret: account.hasTotp ? MASK : '',
        appPassword: account.hasAppPassword ? MASK : '',
        note: account.note,
      })
    } else {
      setForm(EMPTY)
    }
  }, [open, account])

  const set = (key: keyof AccountInput) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [key]: e.target.value }))

  const emailValid = useMemo(() => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.email.trim()), [form.email])
  const canSave = emailValid && (editing || form.appPassword.trim().length > 0)

  const submit = async () => {
    setBusy(true)
    setError('')
    try {
      if (account) {
        await api.updateAccount(account.id, form)
        toast('账号已更新', 'success')
      } else {
        await api.createAccount(form)
        toast('账号已新增', 'success')
      }
      onSaved()
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  const runVerify = async () => {
    if (!account) return
    setVerify({ state: 'busy', message: '' })
    try {
      const res = await api.verifyAccount(account.id)
      setVerify({ state: res.ok ? 'ok' : 'fail', message: res.message })
    } catch (err) {
      setVerify({ state: 'fail', message: err instanceof ApiError ? err.message : '验证失败' })
    }
  }

  const copy = async (value: string, label: string) => {
    if (!value || value === MASK) return
    const ok = await copyText(value)
    toast(ok ? `${label} 已复制` : '复制失败', ok ? 'success' : 'error')
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && !busy && onClose()}>
      <DialogContent className="sm:max-w-lg" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>{editing ? '编辑账号' : '新增账号'}</DialogTitle>
          <DialogDescription>
            {editing ? '留空或保持掩码表示不修改对应字段' : '填写 Gmail 账号与应用专用密码'}
          </DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-3">
          <div className="space-y-1">
            <Label htmlFor="email" className="text-[11px] text-muted-foreground">
              邮箱地址
            </Label>
            <Input
              id="email"
              value={form.email}
              onChange={set('email')}
              placeholder="example@gmail.com"
              disabled={busy}
              aria-invalid={Boolean(form.email) && !emailValid}
            />
            {Boolean(form.email) && !emailValid && (
              <p className="text-[10.5px] text-destructive">邮箱格式不正确</p>
            )}
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <SecretField
              id="password"
              label="账号密码"
              value={form.password}
              onChange={set('password')}
              visible={show.password}
              onToggle={() => setShow((s) => ({ ...s, password: !s.password }))}
              onCopy={() => copy(form.password, '密码')}
              disabled={busy}
              hint={editing ? '保持掩码则不修改' : '可选'}
            />
            <SecretField
              id="totp"
              label="2FA 密钥"
              value={form.totpSecret}
              onChange={set('totpSecret')}
              visible={show.totp}
              onToggle={() => setShow((s) => ({ ...s, totp: !s.totp }))}
              onCopy={() => copy(form.totpSecret, '2FA 密钥')}
              disabled={busy}
              hint="Base32，可粘贴 otpauth:// 链接"
            />
          </div>

          <SecretField
            id="app"
            label="应用专用密码 (App Password)"
            value={form.appPassword}
            onChange={set('appPassword')}
            visible={show.app}
            onToggle={() => setShow((s) => ({ ...s, app: !s.app }))}
            onCopy={() => copy(form.appPassword, '应用专用密码')}
            disabled={busy}
            hint={
              editing
                ? '保持掩码则不修改；16 位，空格会被自动移除'
                : 'Google 账号 16 位应用专用密码，IMAP 取件必需'
            }
          />

          <div className="space-y-1">
            <Label htmlFor="note" className="text-[11px] text-muted-foreground">
              备注
            </Label>
            <Input
              id="note"
              value={form.note}
              onChange={set('note')}
              disabled={busy}
              placeholder="可选"
            />
          </div>

          {error && (
            <div className="rounded-md bg-destructive/8 px-2.5 py-1.5 text-[11.5px] text-destructive">
              {error}
            </div>
          )}
          {verify.state !== 'idle' && (
            <div
              className={
                verify.state === 'ok'
                  ? 'flex items-start gap-1.5 rounded-md bg-emerald-500/10 px-2.5 py-1.5 text-[11.5px] text-emerald-700 dark:text-emerald-400'
                  : verify.state === 'fail'
                    ? 'rounded-md bg-destructive/8 px-2.5 py-1.5 text-[11.5px] text-destructive'
                    : 'rounded-md bg-primary/8 px-2.5 py-1.5 text-[11.5px] text-primary'
              }
            >
              {verify.state === 'busy' ? (
                <>
                  <Loader2 className="mt-px size-3 shrink-0 animate-spin" />
                  正在验证 IMAP 登录…
                </>
              ) : (
                verify.message
              )}
            </div>
          )}

          {editing && (
            <>
              <div className="h-px bg-border" />
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  size="xs"
                  onClick={runVerify}
                  disabled={verify.state === 'busy' || !account?.hasAppPassword}
                >
                  {verify.state === 'busy' ? (
                    <Loader2 className="size-3 animate-spin" />
                  ) : (
                    <ShieldCheck className="size-3" />
                  )}
                  验证 IMAP 登录
                </Button>
                <span className="text-[10.5px] text-muted-foreground">
                  使用已保存的应用专用密码连接 imap.gmail.com
                </span>
              </div>
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose} disabled={busy}>
            取消
          </Button>
          <Button size="sm" onClick={submit} disabled={busy || !canSave}>
            {busy && <Loader2 className="size-3.5 animate-spin" />}
            {busy ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SecretField({
  id,
  label,
  value,
  onChange,
  visible,
  onToggle,
  onCopy,
  disabled,
  hint,
}: {
  id: string
  label: string
  value: string
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  visible: boolean
  onToggle: () => void
  onCopy: () => void
  disabled?: boolean
  hint?: string
}) {
  const masked = value === MASK
  return (
    <div className="space-y-1">
      <Label htmlFor={id} className="text-[11px] text-muted-foreground">
        {label}
      </Label>
      <div className="relative">
        <Input
          id={id}
          value={value}
          onChange={onChange}
          disabled={disabled}
          type={visible ? 'text' : 'password'}
          className="pr-14 font-mono text-[11.5px]"
        />
        <div className="absolute top-1/2 right-1 flex -translate-y-1/2 items-center">
          <button
            type="button"
            onClick={onToggle}
            disabled={masked}
            aria-label={visible ? '隐藏' : '显示'}
            className="rounded p-1 text-muted-foreground/60 transition-colors hover:text-foreground disabled:opacity-40"
          >
            {visible ? <EyeOff className="size-3" /> : <Eye className="size-3" />}
          </button>
          <button
            type="button"
            onClick={onCopy}
            disabled={!value || masked}
            aria-label="复制"
            className="rounded p-1 text-muted-foreground/60 transition-colors hover:text-foreground disabled:opacity-40"
          >
            <Copy className="size-3" />
          </button>
        </div>
      </div>
      {hint && <p className="text-[10px] text-muted-foreground/80">{hint}</p>}
    </div>
  )
}

/** Exported for the settings page to show the same key icon. */
export const AccountKeyIcon = KeyRound
