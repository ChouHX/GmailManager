import { useState, type ReactNode } from 'react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import {
  Check,
  ChevronDown,
  Inbox,
  Menu,
  Settings2,
  ShieldCheck,
  Users,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useAuth } from './AuthProvider'
import { useThemeMode, type ThemeMode } from './ThemeProvider'
import { useFontScale, FONT_MIN, FONT_MAX, FONT_DEFAULT } from './FontScaleProvider'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import ProfileDropdown from '@/components/kokonutui/profile-dropdown'

export type Route = 'accounts' | 'fetch' | 'settings'

const NAV: { key: Route; label: string; hint: string; icon: LucideIcon }[] = [
  { key: 'accounts', label: '账号管理', hint: '导入 / 编辑 / 导出', icon: Users },
  { key: 'fetch', label: '账号取件', hint: 'IMAP 批量 / 单取', icon: Inbox },
  { key: 'settings', label: '设置', hint: '部署与安全', icon: Settings2 },
]

interface ShellProps {
  route: Route
  onNavigate: (route: Route) => void
  children: ReactNode
}

/** Desktop top-bar tab with a sliding active pill. */
function NavTab({
  item,
  active,
  onClick,
}: {
  item: (typeof NAV)[number]
  active: boolean
  onClick: () => void
}) {
  const Icon = item.icon
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'relative flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[12.5px] transition-colors',
        active ? 'text-foreground' : 'text-muted-foreground hover:text-foreground',
      )}
    >
      {active && (
        <motion.span
          layoutId="nav-active"
          transition={{ type: 'spring', duration: 0.32, bounce: 0.1 }}
          className="absolute inset-0 rounded-md bg-accent"
        />
      )}
      <Icon
        className={cn(
          'relative z-10 size-3.5 shrink-0',
          active ? 'text-primary' : 'text-muted-foreground/80',
        )}
      />
      <span className={cn('relative z-10', active && 'font-medium')}>{item.label}</span>
    </button>
  )
}

/** Mobile nav: the current section collapses into a dropdown. */
function MobileNav({ route, onNavigate }: { route: Route; onNavigate: (r: Route) => void }) {
  const [open, setOpen] = useState(false)
  const current = NAV.find((n) => n.key === route) ?? NAV[0]
  const CurrentIcon = current.icon

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`当前：${current.label}，切换页面`}
          className="flex h-8 min-w-0 items-center gap-1.5 rounded-lg border border-border bg-card px-2 text-[12.5px] transition-colors active:bg-accent"
        >
          <CurrentIcon className="size-3.5 shrink-0 text-primary" />
          <span className="truncate font-medium">{current.label}</span>
          <ChevronDown
            className={cn(
              'size-3.5 shrink-0 text-muted-foreground transition-transform duration-200',
              open && 'rotate-180',
            )}
          />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-[13.5rem]">
        <DropdownMenuLabel>导航</DropdownMenuLabel>
        {NAV.map((item) => {
          const Icon = item.icon
          const active = item.key === route
          return (
            <DropdownMenuItem
              key={item.key}
              onSelect={() => onNavigate(item.key)}
              className={cn('py-2.5', active && 'bg-accent/60')}
            >
              <Icon className={cn('size-4', active ? 'text-primary' : 'text-muted-foreground')} />
              <span className="min-w-0 flex-1 leading-tight">
                <span className={cn('block', active ? 'font-medium' : 'font-normal')}>
                  {item.label}
                </span>
                <span className="block text-[10px] text-muted-foreground">{item.hint}</span>
              </span>
              {active && <Check className="size-3.5 text-primary" />}
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function AppShell({ route, onNavigate, children }: ShellProps) {
  const { user, logout } = useAuth()
  const { mode, setMode } = useThemeMode()
  const { size, setSize, reset } = useFontScale()
  const [confirmLogout, setConfirmLogout] = useState(false)
  const reduce = useReducedMotion()

  const username = user?.username ?? 'admin'

  return (
    <div className="flex h-full flex-col overflow-hidden bg-background">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-card px-3">
        {/* Brand */}
        <div className="flex min-w-0 items-center gap-2">
          <div className="grid size-6 shrink-0 place-items-center rounded-md bg-primary text-[9.5px] font-bold text-primary-foreground">
            GM
          </div>
          <span className="hidden truncate text-[12.5px] font-semibold tracking-tight sm:inline">
            Gmail Manager
          </span>
        </div>

        <div className="mx-0.5 hidden h-5 w-px bg-border md:block" />

        {/* Desktop nav */}
        <nav className="hidden items-center gap-0.5 md:flex" aria-label="主导航">
          {NAV.map((item) => (
            <NavTab
              key={item.key}
              item={item}
              active={route === item.key}
              onClick={() => onNavigate(item.key)}
            />
          ))}
        </nav>

        {/* Mobile nav */}
        <div className="md:hidden">
          <MobileNav route={route} onNavigate={onNavigate} />
        </div>

        <div className="flex-1" />

        {user?.defaultPasswordInUse && (
          <Badge variant="warning" className="hidden gap-1 sm:inline-flex">
            <ShieldCheck className="size-2.5" />
            默认密码
          </Badge>
        )}

        <span className="hidden text-[10.5px] text-muted-foreground/80 lg:inline">
          {new Date().toLocaleDateString('zh-CN')}
        </span>

        <ProfileDropdown
          accounts={[{ id: 'admin', name: username, email: '管理员账户', plan: 'ADMIN' }]}
          defaultAccountId="admin"
          theme={mode}
          onThemeChange={(t) => setMode(t as ThemeMode)}
          fontSize={{
            size,
            min: FONT_MIN,
            max: FONT_MAX,
            base: FONT_DEFAULT,
            onChange: setSize,
            onReset: reset,
          }}
          items={[
            {
              label: '设置',
              icon: <Settings2 className="size-3.5" />,
              onSelect: () => onNavigate('settings'),
            },
          ]}
          onSignOut={() => setConfirmLogout(true)}
          className="[&>button]:max-w-[9.5rem]"
        />
      </header>

      <main className="flex-1 overflow-auto p-3 sm:p-4">
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={route}
            initial={reduce ? false : { opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={reduce ? undefined : { opacity: 0, y: -4 }}
            transition={{ duration: 0.2, ease: [0.23, 1, 0.32, 1] }}
          >
            {children}
          </motion.div>
        </AnimatePresence>
      </main>

      <Dialog open={confirmLogout} onOpenChange={setConfirmLogout}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>退出登录</DialogTitle>
            <DialogDescription>确认退出当前会话？</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => setConfirmLogout(false)}>
              取消
            </Button>
            <Button
              size="sm"
              onClick={() => {
                setConfirmLogout(false)
                void logout()
              }}
            >
              退出
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

/** Exposed so pages can show a compact "menu" affordance if needed. */
export const NavMenuIcon = Menu
