"use client";

/**
 * @author: @dorianbaffier
 * @description: Account Menu
 * @version: 2.0.0
 * @date: 2026-10-14
 * @license: MIT
 * @website: https://kokonutui.com
 * @github: https://github.com/kokonut-labs/kokonutui
 */

import {
  Check,
  ChevronLeft,
  ChevronRight,
  ChevronsUpDown,
  CreditCard,
  LogOut,
  Minus,
  Monitor,
  Moon,
  Plus,
  Settings,
  Sun,
  User,
} from "lucide-react";
import {
  animate,
  motion,
  type Transition,
  useMotionTemplate,
  useMotionValue,
  useReducedMotion,
} from "motion/react";
import {
  type HTMLAttributes,
  type KeyboardEvent,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { cn } from "@/lib/utils";

export interface Account {
  id: string;
  name: string;
  email: string;
  /** Shown in the avatar. Defaults to the first letters of `name`. */
  initials?: string;
  /** Plain label shown in the menu header, such as "Pro". */
  plan?: string;
}

export type AccountTheme = "light" | "dark" | "system";

export interface ProfileMenuItem {
  label: string;
  icon?: ReactNode;
  /** Quiet text on the right, such as a plan name or a shortcut. */
  value?: string;
  /** Renders the item as a link. Without it the item is a button. */
  href?: string;
  /** Opens `href` in a new tab. */
  external?: boolean;
  onSelect?: () => void;
}

interface ProfileDropdownProps
  extends Omit<HTMLAttributes<HTMLDivElement>, "onChange"> {
  accounts?: Account[];
  defaultAccountId?: string;
  onAccountChange?: (account: Account) => void;
  onAddAccount?: () => void;
  /** Menu entries between the settings rows and Switch account. */
  items?: ProfileMenuItem[];
  /** Controlled theme. Leave it out to keep the choice in local state. */
  theme?: AccountTheme;
  defaultTheme?: AccountTheme;
  onThemeChange?: (theme: AccountTheme) => void;
  /** UI font size control. Omit to hide the row. */
  fontSize?: FontSizeConfig;
  /** Called when the sign-out item is clicked. */
  onSignOut?: () => void;
}

const SAMPLE_ACCOUNTS: Account[] = [
  { id: "personal", name: "Maya Ortega", email: "maya@ortega.me", plan: "Pro" },
  {
    id: "studio",
    name: "Fieldwork Studio",
    email: "team@fieldwork.studio",
    plan: "Team",
  },
  {
    id: "side",
    name: "Night Shift",
    email: "hello@nightshift.dev",
    plan: "Free",
  },
];

const SAMPLE_ITEMS: ProfileMenuItem[] = [
  { label: "Profile", icon: <User className="size-4" /> },
  { label: "Settings", icon: <Settings className="size-4" />, value: "⌘," },
  { label: "Billing", icon: <CreditCard className="size-4" /> },
];

export interface FontSizeConfig {
  /** Current base font size in px. */
  size: number;
  min: number;
  max: number;
  /** Design baseline; the "reset" value. */
  base: number;
  onChange: (size: number) => void;
  onReset?: () => void;
}

const THEMES = [
  { id: "light", label: "浅色", Icon: Sun },
  { id: "dark", label: "深色", Icon: Moon },
  { id: "system", label: "跟随系统", Icon: Monitor },
] as const;

const AVATAR_TONES = [
  "bg-primary text-primary-foreground",
  "bg-muted text-foreground/80",
  "bg-muted text-muted-foreground ring-1 ring-border ring-inset",
];

const WHITESPACE = /\s+/;
const PANEL_W = 264;
const RADIUS = 16;
const SPRING: Transition = { type: "spring", duration: 0.3, bounce: 0 };
const INDICATOR_SPRING: Transition = {
  type: "spring",
  duration: 0.25,
  bounce: 0,
};
const INSTANT: Transition = { duration: 0 };
const EASE_OUT = [0.23, 1, 0.32, 1] as const;
const NAV_KEYS = new Set([
  "ArrowDown",
  "ArrowUp",
  "ArrowLeft",
  "ArrowRight",
  "Home",
  "End",
]);

const ITEM_CLASS =
  "flex w-full cursor-default items-center gap-2.5 rounded-[10px] px-2.5 py-2 text-left text-sm text-foreground/80 outline-none transition-colors duration-100 focus:bg-accent focus:text-accent-foreground";

type Phase = "closed" | "open" | "closing";
type Page = "main" | "accounts";
type Via = "pointer" | "keyboard";

function initialsOf(account: Account) {
  if (account.initials) {
    return account.initials;
  }
  return account.name
    .split(WHITESPACE)
    .slice(0, 2)
    .map((part) => part[0] ?? "")
    .join("")
    .toUpperCase();
}

function viaOf(event: MouseEvent): Via {
  // A click fired by Enter or Space carries no pointer detail.
  return event.detail === 0 ? "keyboard" : "pointer";
}

function menuItems(root: HTMLElement | null) {
  if (!root) {
    return [];
  }
  return Array.from(root.querySelectorAll<HTMLElement>("[data-menu-item]"));
}

function sideTarget(items: HTMLElement[], current: HTMLElement, step: number) {
  const row = items.filter((item) => item.dataset.row === current.dataset.row);
  if (row.length < 2) {
    return;
  }
  const index = row.indexOf(current);
  return row[(index + step + row.length) % row.length];
}

function rowTarget(items: HTMLElement[], current: HTMLElement, step: number) {
  const rows = [...new Set(items.map((item) => item.dataset.row))];
  const index = rows.indexOf(current.dataset.row);
  const next = rows[(index + step + rows.length) % rows.length];
  const candidates = items.filter((item) => item.dataset.row === next);
  return (
    candidates.find((item) => item.getAttribute("aria-checked") === "true") ??
    candidates[0]
  );
}

/** Where arrow keys move focus: rows go up and down, radios left and right. */
function nextTarget(
  items: HTMLElement[],
  active: Element | null,
  key: string
): HTMLElement | undefined {
  if (key === "Home") {
    return items[0];
  }
  if (key === "End") {
    return items.at(-1);
  }
  const current = items.find((item) => item === active);
  if (!current) {
    return key === "ArrowUp" ? items.at(-1) : items[0];
  }
  if (key === "ArrowLeft" || key === "ArrowRight") {
    return sideTarget(items, current, key === "ArrowRight" ? 1 : -1);
  }
  return rowTarget(items, current, key === "ArrowDown" ? 1 : -1);
}

function useControllable<T>(
  value: T | undefined,
  fallback: T,
  onChange?: (next: T) => void
) {
  const [local, setLocal] = useState(fallback);
  const set = (next: T) => {
    setLocal(next);
    onChange?.(next);
  };
  return [value ?? local, set] as const;
}

function Avatar({
  account,
  tone,
  className,
}: {
  account: Account;
  tone: number;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "relative grid shrink-0 place-items-center rounded-full font-medium tracking-wide select-none",
        AVATAR_TONES[tone % AVATAR_TONES.length],
        className
      )}
    >
      {initialsOf(account)}
    </span>
  );
}

/**
 * The avatar, name and email. Rendered in the trigger while the menu is
 * closed and in the menu header while it is open; shared layout ids let the
 * three pieces travel between the two places.
 */
function Identity({
  uid,
  shared,
  account,
  tone,
  transition,
  stacked,
  showEmail = stacked,
}: {
  uid: string;
  shared: boolean;
  account: Account;
  tone: number;
  transition: Transition;
  stacked: boolean;
  /** Renders the address under the name. Defaults to `stacked`. */
  showEmail?: boolean;
}) {
  const hidden = shared ? undefined : "invisible";
  return (
    <span
      className={cn(
        "flex min-w-0",
        stacked ? "flex-col items-start gap-3" : "items-center gap-2.5"
      )}
    >
      <motion.span
        className={cn("relative z-20 block", hidden)}
        key={shared ? "avatar-shared" : "avatar-static"}
        layoutId={shared ? `${uid}-avatar` : undefined}
        transition={transition}
      >
        <Avatar
          account={account}
          className={stacked ? "size-8" : "size-6 text-[10px]"}
          tone={tone}
        />
      </motion.span>
      <span className="flex min-w-0 flex-col text-left">
        <motion.span
          className={cn(
            "relative z-20 block truncate font-medium text-sm text-foreground leading-5 dark:text-foreground",
            hidden
          )}
          key={shared ? "name-shared" : "name-static"}
          layoutId={shared ? `${uid}-name` : undefined}
          transition={transition}
        >
          {account.name}
        </motion.span>
        <motion.span
          className={cn(
            "relative z-20 block truncate text-xs text-muted-foreground leading-4 dark:text-muted-foreground/70",
            showEmail ? "" : "hidden",
            hidden
          )}
          key={shared ? "email-shared" : "email-static"}
          layoutId={shared ? `${uid}-email` : undefined}
          transition={transition}
        >
          {account.email}
        </motion.span>
      </span>
    </span>
  );
}

function Separator() {
  return (
    <hr className="mx-2.5 my-1.5 h-px border-0 bg-muted dark:bg-muted" />
  );
}

function SignOutItem({
  onSelect,
}: {
  onSelect: (event: MouseEvent) => void;
}) {
  return (
    <button
      className={cn(ITEM_CLASS, "text-muted-foreground hover:text-destructive")}
      data-menu-item=""
      data-row="sign-out"
      onClick={onSelect}
      role="menuitem"
      tabIndex={-1}
      type="button"
    >
      <LogOut className="size-4" />
      <span className="flex-1">退出登录</span>
    </button>
  );
}

function ThemeRow({
  uid,
  theme,
  onChange,
  reduced,
}: {
  uid: string;
  theme: AccountTheme;
  onChange: (theme: AccountTheme) => void;
  reduced: boolean;
}) {
  return (
    <fieldset
      aria-label="主题"
      className="m-0 flex min-w-0 items-center justify-between border-0 py-1 pr-1 pl-2.5"
    >
      <span className="text-xs text-muted-foreground">主题</span>
      <span className="flex rounded-[10px] bg-muted p-0.5 dark:bg-muted">
        {THEMES.map(({ id, label, Icon }) => {
          const checked = id === theme;
          return (
            <button
              aria-checked={checked}
              aria-label={label}
              className="relative grid h-7 w-9 place-items-center rounded-lg text-muted-foreground outline-none transition-colors duration-150 hover:text-foreground/90 focus-visible:ring-2 focus-visible:ring-ring aria-checked:text-foreground dark:text-muted-foreground/70 dark:aria-checked:text-foreground dark:hover:text-muted-foreground/50"
              data-menu-item=""
              data-row="theme"
              key={id}
              onClick={() => onChange(id)}
              role="menuitemradio"
              tabIndex={-1}
              type="button"
            >
              {checked ? (
                <motion.span
                  className="absolute inset-0 rounded-lg bg-card shadow-[0_1px_2px_rgb(0_0_0/0.08)] dark:bg-muted"
                  layoutId={`${uid}-theme`}
                  transition={reduced ? INSTANT : INDICATOR_SPRING}
                />
              ) : null}
              <Icon className="relative size-3.5" />
            </button>
          );
        })}
      </span>
    </fieldset>
  );
}

function FontSizeRow({ config }: { config: FontSizeConfig }) {
  const { size, min, max, base, onChange, onReset } = config;
  const dec = () => onChange(Math.max(min, size - 1));
  const inc = () => onChange(Math.min(max, size + 1));
  const stepBtn =
    "grid size-6 place-items-center rounded-md text-muted-foreground outline-none transition-colors hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40";

  return (
    <div
      className="flex min-w-0 items-center justify-between gap-2 rounded-[10px] py-1 pr-1 pl-2.5 outline-none focus-visible:ring-2 focus-visible:ring-ring"
      data-menu-item=""
      data-row="font"
      tabIndex={-1}
    >
      <span className="flex shrink-0 items-baseline gap-1.5 text-xs text-muted-foreground">
        字体
        <button
          className="rounded font-mono text-[10px] text-muted-foreground/70 underline-offset-2 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring disabled:invisible"
          disabled={size === base}
          onClick={() => (onReset ? onReset() : onChange(base))}
          tabIndex={-1}
          title={`恢复默认 ${base}px`}
          type="button"
        >
          {size}px
        </button>
      </span>

      <span className="flex items-center gap-0.5 rounded-[10px] bg-muted p-0.5">
        <button
          aria-label="减小字体"
          className={stepBtn}
          disabled={size <= min}
          onClick={dec}
          tabIndex={-1}
          type="button"
        >
          <Minus className="size-3.5" />
        </button>

        {/* A thin range slider: dragging is faster, the buttons are precise. */}
        <input
          aria-label="字体大小"
          className="h-1 w-16 cursor-pointer appearance-none rounded-full bg-border accent-[var(--primary)] outline-none focus-visible:ring-2 focus-visible:ring-ring"
          max={max}
          min={min}
          onChange={(e) => onChange(Number(e.target.value))}
          step={1}
          tabIndex={-1}
          type="range"
          value={size}
        />

        <button
          aria-label="增大字体"
          className={stepBtn}
          disabled={size >= max}
          onClick={inc}
          tabIndex={-1}
          type="button"
        >
          <Plus className="size-3.5" />
        </button>
      </span>
    </div>
  );
}

function LinkItem({
  item,
  index,
  onSelect,
}: {
  item: ProfileMenuItem;
  index: number;
  onSelect: (event: MouseEvent, run?: () => void) => void;
}) {
  const inner = (
    <>
      <span className="text-muted-foreground/70 dark:text-muted-foreground">{item.icon}</span>
      <span className="flex-1">{item.label}</span>
      {item.value ? (
        <span className="text-xs text-muted-foreground/70 dark:text-muted-foreground">
          {item.value}
        </span>
      ) : null}
    </>
  );
  const common = {
    className: ITEM_CLASS,
    "data-menu-item": "",
    "data-row": `item-${index}`,
    onClick: (event: MouseEvent) => onSelect(event, item.onSelect),
    role: "menuitem",
    tabIndex: -1,
  } as const;
  if (item.href) {
    return (
      <a
        {...common}
        href={item.href}
        {...(item.external && { rel: "noopener noreferrer", target: "_blank" })}
      >
        {inner}
      </a>
    );
  }
  return (
    <button {...common} type="button">
      {inner}
    </button>
  );
}

export default function ProfileDropdown({
  accounts = SAMPLE_ACCOUNTS,
  defaultAccountId,
  onAccountChange,
  onAddAccount,
  items = SAMPLE_ITEMS,
  theme: themeProp,
  defaultTheme = "system",
  onThemeChange,
  fontSize,
  onSignOut,
  className,
  ...props
}: ProfileDropdownProps) {
  const uid = useId();
  const reduced = useReducedMotion() === true;
  const [phase, setPhase] = useState<Phase>("closed");
  const [instant, setInstant] = useState(false);
  const [page, setPage] = useState<Page>("main");
  const [openSeq, setOpenSeq] = useState(0);
  const [accountId, setAccountId] = useState(
    defaultAccountId ?? accounts[0]?.id
  );
  const [theme, setTheme] = useControllable(
    themeProp,
    defaultTheme,
    onThemeChange
  );

  const wrapperRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const mainRef = useRef<HTMLDivElement>(null);
  const accountsRef = useRef<HTMLDivElement>(null);
  const phaseRef = useRef<Phase>("closed");
  phaseRef.current = phase;
  const openIntent = useRef<{
    focus: "first" | "last" | "menu";
    resume: boolean;
  }>({ focus: "menu", resume: false });
  const pageFocus = useRef<"first" | "switch" | null>(null);
  const closeSeq = useRef(0);
  const targetH = useRef(0);

  // The panel's visible box. The two surface layers (border and fill) and the
  // content all clip to it, so the menu grows out of the trigger's own
  // rectangle without any layout work per frame.
  const w = useMotionValue(0);
  const h = useMotionValue(0);
  const pageX = useMotionValue(0);
  const bodyOpacity = useMotionValue(1);
  const outerClip = useMotionTemplate`inset(0px 0px calc(100% - ${h}px) calc(100% - ${w}px) round ${RADIUS}px)`;
  const innerClip = useMotionTemplate`inset(1px 1px calc(100% - ${h}px + 1px) calc(100% - ${w}px + 1px) round ${RADIUS - 1}px)`;
  const pageTransform = useMotionTemplate`translate3d(${pageX}px, 0, 0)`;

  const account = accounts.find((entry) => entry.id === accountId) ??
    accounts[0] ?? { id: "guest", name: "Guest", email: "" };
  const tone = Math.max(0, accounts.indexOf(account));
  const others = accounts.filter((entry) => entry.id !== account.id);
  const sharedTransition = instant ? INSTANT : SPRING;
  const sharedInPanel = phase === "open";
  const menuId = `${uid}-menu`;

  const pageEl = (target: Page) =>
    target === "main" ? mainRef.current : accountsRef.current;

  const openMenu = (via: Via, focus: "first" | "last" | "menu") => {
    if (phaseRef.current === "open") {
      return;
    }
    openIntent.current = { focus, resume: phaseRef.current === "closing" };
    closeSeq.current += 1;
    setInstant(via === "keyboard" || reduced);
    setPage("main");
    setPhase("open");
    setOpenSeq((seq) => seq + 1);
  };

  const closeMenu = (via: Via, returnFocus: boolean) => {
    if (phaseRef.current !== "open") {
      return;
    }
    const trigger = triggerRef.current;
    if (returnFocus) {
      trigger?.focus({ preventScroll: true });
    }
    if (via === "keyboard" || reduced || !trigger) {
      setInstant(true);
      setPhase("closed");
      return;
    }
    setInstant(false);
    setPhase("closing");
    const seq = ++closeSeq.current;
    animate(bodyOpacity, 0, { duration: 0.12, ease: EASE_OUT });
    animate(w, trigger.offsetWidth, SPRING);
    animate(h, trigger.offsetHeight, SPRING).then(() => {
      if (closeSeq.current === seq) {
        setPhase("closed");
      }
    });
  };

  const goTo = (target: Page) => {
    pageFocus.current = target === "accounts" ? "first" : "switch";
    setPage(target);
  };

  const select = (event: MouseEvent, run?: () => void) => {
    run?.();
    closeMenu(viaOf(event), true);
  };

  // Opening: size the clip to the trigger, then spring it out to the menu.
  // A keyboard open skips straight to the end.
  // biome-ignore lint/correctness/useExhaustiveDependencies: runs once per open
  useLayoutEffect(() => {
    const trigger = triggerRef.current;
    const main = mainRef.current;
    if (openSeq === 0 || phaseRef.current !== "open" || !trigger || !main) {
      return;
    }
    pageX.jump(0);
    targetH.current = main.offsetHeight;
    if (instant) {
      w.jump(PANEL_W);
      h.jump(targetH.current);
      bodyOpacity.jump(1);
    } else {
      if (!openIntent.current.resume) {
        w.jump(trigger.offsetWidth);
        h.jump(trigger.offsetHeight);
        bodyOpacity.jump(0);
      }
      animate(w, PANEL_W, SPRING);
      animate(h, targetH.current, SPRING);
      animate(bodyOpacity, 1, { duration: 0.2, delay: 0.05, ease: EASE_OUT });
    }
    const list = menuItems(main);
    const targets = {
      first: list[0],
      last: list.at(-1),
      menu: main.querySelector<HTMLElement>("[role=menu]"),
    };
    targets[openIntent.current.focus]?.focus({ preventScroll: true });
  }, [openSeq]);

  // Switching pages: push sideways and spring the height to the new page.
  // biome-ignore lint/correctness/useExhaustiveDependencies: keyed on page
  useEffect(() => {
    const el = pageEl(page);
    if (phaseRef.current !== "open" || !el) {
      return;
    }
    const x = page === "main" ? 0 : -PANEL_W;
    targetH.current = el.offsetHeight;
    if (reduced) {
      pageX.jump(x);
      h.jump(targetH.current);
    } else {
      animate(pageX, x, SPRING);
      animate(h, targetH.current, SPRING);
    }
    const intent = pageFocus.current;
    pageFocus.current = null;
    if (intent === "first") {
      const list = menuItems(el);
      const checked = list.find(
        (item) => item.getAttribute("aria-checked") === "true"
      );
      (checked ?? list[0])?.focus({ preventScroll: true });
    } else if (intent === "switch") {
      el.querySelector<HTMLElement>("[data-switch]")?.focus({
        preventScroll: true,
      });
    }
  }, [page]);

  // Keep the height in step if a page's content changes size while open.
  // biome-ignore lint/correctness/useExhaustiveDependencies: observers read refs
  useEffect(() => {
    if (phase !== "open") {
      return;
    }
    const observer = new ResizeObserver(() => {
      const el = pageEl(page);
      if (!el || phaseRef.current !== "open") {
        return;
      }
      const next = el.offsetHeight;
      if (Math.abs(next - targetH.current) < 0.5) {
        return;
      }
      targetH.current = next;
      animate(h, next, reduced ? INSTANT : SPRING);
    });
    for (const el of [mainRef.current, accountsRef.current]) {
      if (el) {
        observer.observe(el);
      }
    }
    return () => observer.disconnect();
  }, [phase, page]);

  // A press anywhere outside closes, with the morph.
  // biome-ignore lint/correctness/useExhaustiveDependencies: closeMenu reads refs
  useEffect(() => {
    if (phase !== "open") {
      return;
    }
    const onDown = (event: globalThis.PointerEvent) => {
      if (!wrapperRef.current?.contains(event.target as Node)) {
        closeMenu("pointer", false);
      }
    };
    document.addEventListener("pointerdown", onDown, true);
    return () => document.removeEventListener("pointerdown", onDown, true);
  }, [phase]);

  const onPanelKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape" || event.key === "Tab") {
      event.preventDefault();
      closeMenu("keyboard", true);
      return;
    }
    if (page === "accounts" && event.key === "Backspace") {
      event.preventDefault();
      goTo("main");
      return;
    }
    if (!NAV_KEYS.has(event.key)) {
      return;
    }
    const focused = document.activeElement;
    if (focused instanceof HTMLInputElement && focused.type === "range") {
      // Native range behaviour: arrows adjust the value.
      return;
    }
    event.preventDefault();
    const active = document.activeElement;
    const target = nextTarget(menuItems(pageEl(page)), active, event.key);
    if (target) {
      target.focus({ preventScroll: true });
    } else if (page === "accounts" && event.key === "ArrowLeft") {
      goTo("main");
    } else if (
      event.key === "ArrowRight" &&
      active instanceof HTMLElement &&
      active.dataset.switch !== undefined
    ) {
      goTo("accounts");
    }
  };

  // Hovering an item focuses it, so pointer and keyboard share one highlight.
  const onPanelPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (event.pointerType !== "mouse") {
      return;
    }
    const item = (event.target as HTMLElement).closest<HTMLElement>(
      "[data-menu-item]"
    );
    if (item && document.activeElement !== item) {
      item.focus({ preventScroll: true });
    }
  };

  return (
    <div
      className={cn(
        "relative isolate inline-block",
        phase !== "closed" && "z-50",
        className
      )}
      ref={wrapperRef}
      {...props}
    >
      <button
        aria-controls={sharedInPanel ? menuId : undefined}
        aria-expanded={sharedInPanel}
        aria-haspopup="menu"
        className="flex items-center gap-2 rounded-lg border border-transparent px-1.5 py-1 text-left outline-none transition-colors duration-150 hover:border-border hover:bg-accent/70 focus-visible:ring-2 focus-visible:ring-ring/40"
        onClick={(event) => {
          const via = viaOf(event);
          openMenu(via, via === "keyboard" ? "first" : "menu");
        }}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            openMenu("keyboard", event.key === "ArrowDown" ? "first" : "last");
          }
        }}
        ref={triggerRef}
        type="button"
      >
        <span className="min-w-0">
          <Identity
            account={account}
            shared={!sharedInPanel}
            stacked={false}
            tone={tone}
            transition={sharedTransition}
            uid={uid}
          />
        </span>
        <ChevronsUpDown
          aria-hidden="true"
          className={cn(
            "size-3.5 shrink-0 text-muted-foreground/70 transition-opacity duration-150",
            phase !== "closed" && "opacity-0"
          )}
        />
      </button>

      {phase === "closed" ? null : (
        <div
          className="pointer-events-none absolute top-0 right-0 z-10"
          style={{ width: PANEL_W }}
        >
          <div
            aria-hidden="true"
            className="absolute inset-0 [filter:drop-shadow(0_12px_24px_rgb(0_0_0/0.08))_drop-shadow(0_2px_4px_rgb(0_0_0/0.04))] dark:[filter:drop-shadow(0_12px_28px_rgb(0_0_0/0.5))]"
          >
            <motion.div
              className="absolute inset-0 bg-border"
              style={{ clipPath: outerClip }}
            />
            <motion.div
              className="absolute inset-0 bg-card dark:bg-card"
              style={{ clipPath: innerClip }}
            />
          </div>
          <motion.div
            className={cn("relative", sharedInPanel && "pointer-events-auto")}
            onKeyDown={onPanelKeyDown}
            onPointerMove={onPanelPointerMove}
            style={{ clipPath: innerClip }}
          >
            <motion.div
              className="flex w-max items-start"
              style={{ transform: pageTransform }}
            >
              <div
                className={cn(
                  "shrink-0 p-1.5 transition-opacity duration-300 ease-[cubic-bezier(0.23,1,0.32,1)]",
                  page !== "main" && "opacity-0"
                )}
                inert={page !== "main" ? "" : undefined}
                ref={mainRef}
                style={{ width: PANEL_W }}
              >
                <div className="flex items-center gap-2.5 px-2.5 pt-2.5 pb-2.5">
                  <Identity
                    account={account}
                    shared={sharedInPanel}
                    stacked={false}
                    showEmail
                    tone={tone}
                    transition={sharedTransition}
                    uid={uid}
                  />
                  {account.plan ? (
                    <motion.span
                      className="ml-auto shrink-0 rounded-md border border-border px-1.5 py-0.5 text-[10px] font-medium tracking-wide text-muted-foreground"
                      style={{ opacity: bodyOpacity }}
                    >
                      {account.plan}
                    </motion.span>
                  ) : null}
                </div>
                <motion.div
                  aria-label={account.name}
                  className="outline-none"
                  id={menuId}
                  role="menu"
                  style={{ opacity: bodyOpacity }}
                  tabIndex={-1}
                >
                  <ThemeRow
                    onChange={setTheme}
                    reduced={reduced}
                    theme={theme}
                    uid={uid}
                  />
                  {fontSize ? (
                    <FontSizeRow config={fontSize} />
                  ) : null}
                  <Separator />
                  {items.map((item, index) => (
                    <LinkItem
                      index={index}
                      item={item}
                      key={item.label}
                      onSelect={select}
                    />
                  ))}
                  {accounts.length > 1 ? (
                    <button
                      aria-expanded={page === "accounts"}
                      aria-haspopup="menu"
                      className={ITEM_CLASS}
                      data-menu-item=""
                      data-row="switch"
                      data-switch=""
                      onClick={() => goTo("accounts")}
                      role="menuitem"
                      tabIndex={-1}
                      type="button"
                    >
                      <span className="flex -space-x-1.5">
                        {others.slice(0, 2).map((entry) => (
                          <Avatar
                            account={entry}
                            className="size-5 text-[7px] tracking-normal ring-2 ring-card dark:ring-card"
                            key={entry.id}
                            tone={accounts.indexOf(entry)}
                          />
                        ))}
                      </span>
                      <span className="flex-1">切换账号</span>
                      <ChevronRight className="size-4 text-muted-foreground/70 dark:text-muted-foreground" />
                    </button>
                  ) : null}
                  <Separator />
                  <SignOutItem
                    onSelect={(event) => {
                      // The confirmation dialog is the safety step now, so a
                      // single click is enough to start signing out.
                      onSignOut?.();
                      closeMenu(viaOf(event), true);
                    }}
                  />
                </motion.div>
              </div>

              <div
                className={cn(
                  "shrink-0 p-1.5 transition-opacity duration-300 ease-[cubic-bezier(0.23,1,0.32,1)]",
                  page !== "accounts" && "opacity-0"
                )}
                inert={page !== "accounts" ? "" : undefined}
                ref={accountsRef}
                style={{ width: PANEL_W }}
              >
                <motion.div
                  aria-label="切换账号"
                  className="outline-none"
                  role="menu"
                  style={{ opacity: bodyOpacity }}
                  tabIndex={-1}
                >
                  <button
                    className={cn(ITEM_CLASS, "gap-1.5 pl-1.5 font-medium")}
                    data-menu-item=""
                    data-row="back"
                    onClick={() => goTo("main")}
                    role="menuitem"
                    tabIndex={-1}
                    type="button"
                  >
                    <ChevronLeft className="size-4 text-muted-foreground/70 dark:text-muted-foreground" />
                    <span className="sr-only">返回 </span>
                    切换账号
                  </button>
                  <Separator />
                  {accounts.map((entry, index) => {
                    const checked = entry.id === account.id;
                    return (
                      <button
                        aria-checked={checked}
                        className={ITEM_CLASS}
                        data-menu-item=""
                        data-row={`account-${entry.id}`}
                        key={entry.id}
                        onClick={() => {
                          if (!checked) {
                            setAccountId(entry.id);
                            onAccountChange?.(entry);
                          }
                          goTo("main");
                        }}
                        role="menuitemradio"
                        tabIndex={-1}
                        type="button"
                      >
                        <Avatar
                          account={entry}
                          className="size-7"
                          tone={index}
                        />
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="truncate text-foreground dark:text-foreground">
                            {entry.name}
                          </span>
                          <span className="truncate text-xs text-muted-foreground dark:text-muted-foreground/70">
                            {entry.email}
                          </span>
                        </span>
                        {checked ? (
                          <Check className="size-4 text-foreground dark:text-foreground" />
                        ) : null}
                      </button>
                    );
                  })}
                  <Separator />
                  <button
                    className={ITEM_CLASS}
                    data-menu-item=""
                    data-row="add"
                    onClick={(event) => select(event, onAddAccount)}
                    role="menuitem"
                    tabIndex={-1}
                    type="button"
                  >
                    <span className="grid size-7 place-items-center rounded-full border border-border border-dashed text-muted-foreground/70 dark:border-border dark:text-muted-foreground">
                      <Plus className="size-3.5" />
                    </span>
                    添加账号
                  </button>
                </motion.div>
              </div>
            </motion.div>
          </motion.div>
        </div>
      )}
    </div>
  );
}
