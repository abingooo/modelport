/**
 * Centralized platform color definitions.
 *
 * All components that need platform-specific styling should import from here
 * instead of defining their own color mappings.
 */

export type Platform =
  | 'anthropic'
  | 'openai'
  | 'antigravity'
  | 'gemini'
  | 'grok'
  | 'deepseek'
  | 'qwen'
  | 'glm'
  | 'kimi'
  | 'doubao'
  | 'minimax'
  | 'mimo'
  | 'composite'

// ── Badge (bg + text + border, for inline badges with border) ───────
const BADGE: Record<Platform, string> = {
  anthropic: 'bg-amber-500/10 text-amber-700 border-amber-500/30 dark:text-amber-300',
  openai: 'bg-emerald-500/10 text-emerald-700 border-emerald-500/30 dark:text-emerald-300',
  antigravity: 'bg-purple-500/10 text-purple-600 border-purple-500/30 dark:text-purple-400',
  gemini: 'bg-blue-500/10 text-blue-600 border-blue-500/30 dark:text-blue-400',
  grok: 'bg-zinc-800/10 text-zinc-800 border-zinc-800/30 dark:bg-zinc-500/10 dark:text-zinc-200 dark:border-zinc-500/30',
  deepseek: 'bg-indigo-500/10 text-indigo-700 border-indigo-500/30 dark:text-indigo-300',
  qwen: 'bg-violet-500/10 text-violet-700 border-violet-500/30 dark:text-violet-300',
  glm: 'bg-cyan-500/10 text-cyan-800 border-cyan-500/30 dark:text-cyan-300',
  kimi: 'bg-teal-500/10 text-teal-800 border-teal-500/30 dark:text-teal-300',
  doubao: 'bg-sky-500/10 text-sky-800 border-sky-500/30 dark:text-sky-300',
  minimax: 'bg-rose-500/10 text-rose-700 border-rose-500/30 dark:text-rose-300',
  mimo: 'bg-orange-500/10 text-orange-700 border-orange-500/30 dark:text-orange-300',
  composite: 'bg-lime-500/10 text-lime-800 border-lime-500/30 dark:text-lime-300',
}
const BADGE_DEFAULT = 'bg-slate-500/10 text-slate-600 border-slate-500/30 dark:text-slate-400'

// ── Light badge (softer bg, no border) ──────────────────────────────
const BADGE_LIGHT: Record<Platform, string> = {
  anthropic: 'bg-amber-500/10 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300',
  openai: 'bg-emerald-500/10 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300',
  antigravity: 'bg-purple-500/10 text-purple-600 dark:bg-purple-500/10 dark:text-purple-300',
  gemini: 'bg-blue-500/10 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300',
  grok: 'bg-zinc-800/10 text-zinc-800 dark:bg-zinc-500/10 dark:text-zinc-200',
  deepseek: 'bg-indigo-500/10 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300',
  qwen: 'bg-violet-500/10 text-violet-700 dark:bg-violet-500/10 dark:text-violet-300',
  glm: 'bg-cyan-500/10 text-cyan-800 dark:bg-cyan-500/10 dark:text-cyan-300',
  kimi: 'bg-teal-500/10 text-teal-800 dark:bg-teal-500/10 dark:text-teal-300',
  doubao: 'bg-sky-500/10 text-sky-800 dark:bg-sky-500/10 dark:text-sky-300',
  minimax: 'bg-rose-500/10 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300',
  mimo: 'bg-orange-500/10 text-orange-700 dark:bg-orange-500/10 dark:text-orange-300',
  composite: 'bg-lime-500/10 text-lime-800 dark:bg-lime-500/10 dark:text-lime-300',
}

// ── Border ──────────────────────────────────────────────────────────
const BORDER: Record<Platform, string> = {
  anthropic: 'border-amber-500/20 dark:border-amber-500/20',
  openai: 'border-emerald-500/20 dark:border-emerald-500/20',
  antigravity: 'border-purple-500/20 dark:border-purple-500/20',
  gemini: 'border-blue-500/20 dark:border-blue-500/20',
  grok: 'border-zinc-800/20 dark:border-zinc-500/20',
  deepseek: 'border-indigo-500/20 dark:border-indigo-500/20',
  qwen: 'border-violet-500/20 dark:border-violet-500/20',
  glm: 'border-cyan-500/20 dark:border-cyan-500/20',
  kimi: 'border-teal-500/20 dark:border-teal-500/20',
  doubao: 'border-sky-500/20 dark:border-sky-500/20',
  minimax: 'border-rose-500/20 dark:border-rose-500/20',
  mimo: 'border-orange-500/20 dark:border-orange-500/20',
  composite: 'border-lime-500/20 dark:border-lime-500/20',
}
const BORDER_DEFAULT = 'border-gray-200 dark:border-dark-700'

// ── Border strong (higher-contrast platform tint, e.g. plaza group cards) ──
const BORDER_STRONG: Record<Platform, string> = {
  anthropic: 'border-orange-500/35 dark:border-orange-500/30',
  openai: 'border-green-500/35 dark:border-green-500/30',
  antigravity: 'border-purple-500/35 dark:border-purple-500/30',
  gemini: 'border-blue-500/35 dark:border-blue-500/30',
  grok: 'border-zinc-800/35 dark:border-zinc-500/35',
  deepseek: 'border-indigo-500/35 dark:border-indigo-500/30',
  qwen: 'border-violet-500/35 dark:border-violet-500/30',
  glm: 'border-cyan-500/35 dark:border-cyan-500/30',
  kimi: 'border-teal-500/35 dark:border-teal-500/30',
  doubao: 'border-sky-500/35 dark:border-sky-500/30',
  minimax: 'border-rose-500/35 dark:border-rose-500/30',
  mimo: 'border-orange-500/35 dark:border-orange-500/30',
  composite: 'border-cyan-500/35 dark:border-cyan-500/30',
}
const BORDER_STRONG_DEFAULT = 'border-gray-300 dark:border-dark-600'

// ── Accent (single raw color per platform; consumers derive washes/tints
//    from it via CSS color-mix, e.g. plaza paid-price zone) ──
const ACCENT: Record<Platform, string> = {
  anthropic: '#f97316', // orange-500
  openai: '#22c55e', // green-500
  antigravity: '#a855f7', // purple-500
  gemini: '#3b82f6', // blue-500
  grok: '#71717a', // zinc-500
  deepseek: '#4d6bfe',
  qwen: '#7147d8',
  glm: '#0899b8',
  kimi: '#0f8b8d',
  doubao: '#168cff',
  minimax: '#e54868',
  mimo: '#f26822',
  composite: '#06b6d4', // cyan-500
}
const ACCENT_DEFAULT = '#14b8a6' // primary-500 (teal)

// ── Accent bar (gradient) ───────────────────────────────────────────
const ACCENT_BAR: Record<Platform, string> = {
  anthropic: 'bg-gradient-to-r from-amber-400 to-amber-600',
  openai: 'bg-gradient-to-r from-emerald-400 to-emerald-500',
  antigravity: 'bg-gradient-to-r from-purple-400 to-purple-500',
  gemini: 'bg-gradient-to-r from-blue-400 to-blue-500',
  grok: 'bg-gradient-to-r from-zinc-700 to-zinc-900',
  deepseek: 'bg-gradient-to-r from-indigo-500 to-cyan-500',
  qwen: 'bg-gradient-to-r from-violet-500 to-indigo-500',
  glm: 'bg-gradient-to-r from-cyan-500 to-cyan-700',
  kimi: 'bg-gradient-to-r from-teal-500 to-teal-800',
  doubao: 'bg-gradient-to-r from-sky-500 to-sky-700',
  minimax: 'bg-gradient-to-r from-rose-500 to-orange-500',
  mimo: 'bg-gradient-to-r from-orange-500 to-orange-600',
  composite: 'bg-gradient-to-r from-lime-500 to-lime-700',
}
const ACCENT_BAR_DEFAULT = 'bg-gradient-to-r from-primary-400 to-primary-500'

// ── Accent dot ─────────────────────────────────────────────────────
const ACCENT_DOT: Record<Platform, string> = {
  anthropic: 'bg-amber-500',
  openai: 'bg-emerald-500',
  antigravity: 'bg-purple-500',
  gemini: 'bg-blue-500',
  grok: 'bg-zinc-800 dark:bg-zinc-300',
  deepseek: 'bg-indigo-600 dark:bg-indigo-400',
  qwen: 'bg-violet-600 dark:bg-violet-400',
  glm: 'bg-cyan-600 dark:bg-cyan-400',
  kimi: 'bg-teal-600 dark:bg-teal-400',
  doubao: 'bg-sky-600 dark:bg-sky-400',
  minimax: 'bg-rose-600 dark:bg-rose-400',
  mimo: 'bg-orange-600 dark:bg-orange-400',
  composite: 'bg-lime-600 dark:bg-lime-400',
}
const ACCENT_DOT_DEFAULT = 'bg-primary-500 dark:bg-primary-400'

// ── Text (price, icon) ─────────────────────────────────────────────
const TEXT: Record<Platform, string> = {
  anthropic: 'text-amber-700 dark:text-amber-300',
  openai: 'text-emerald-600 dark:text-emerald-400',
  antigravity: 'text-purple-600 dark:text-purple-400',
  gemini: 'text-blue-600 dark:text-blue-400',
  grok: 'text-zinc-800 dark:text-zinc-200',
  deepseek: 'text-indigo-700 dark:text-indigo-300',
  qwen: 'text-violet-700 dark:text-violet-300',
  glm: 'text-cyan-800 dark:text-cyan-300',
  kimi: 'text-teal-800 dark:text-teal-300',
  doubao: 'text-sky-800 dark:text-sky-300',
  minimax: 'text-rose-700 dark:text-rose-300',
  mimo: 'text-orange-700 dark:text-orange-300',
  composite: 'text-lime-800 dark:text-lime-300',
}
const TEXT_DEFAULT = 'text-primary-600 dark:text-primary-400'

// ── Icon (check mark etc.) ──────────────────────────────────────────
const ICON: Record<Platform, string> = {
  anthropic: 'text-amber-600 dark:text-amber-300',
  openai: 'text-emerald-500 dark:text-emerald-400',
  antigravity: 'text-purple-500 dark:text-purple-400',
  gemini: 'text-blue-500 dark:text-blue-400',
  grok: 'text-zinc-800 dark:text-zinc-200',
  deepseek: 'text-indigo-600 dark:text-indigo-300',
  qwen: 'text-violet-600 dark:text-violet-300',
  glm: 'text-cyan-700 dark:text-cyan-300',
  kimi: 'text-teal-700 dark:text-teal-300',
  doubao: 'text-sky-700 dark:text-sky-300',
  minimax: 'text-rose-600 dark:text-rose-300',
  mimo: 'text-orange-600 dark:text-orange-300',
  composite: 'text-lime-700 dark:text-lime-300',
}
const ICON_DEFAULT = 'text-primary-500 dark:text-primary-400'

const ACCENT_HEX: Record<Platform, string> = {
  anthropic: '#d97757',
  openai: '#10a37f',
  antigravity: '#7c3aed',
  gemini: '#4285f4',
  grok: '#27272a',
  deepseek: '#4d6bfe',
  qwen: '#7147d8',
  glm: '#0899b8',
  kimi: '#0f8b8d',
  doubao: '#168cff',
  minimax: '#e54868',
  mimo: '#f26822',
  composite: '#65a30d',
}
const ACCENT_HEX_DEFAULT = '#0d6efd'

// ── Button (solid bg) ───────────────────────────────────────────────
const BUTTON: Record<Platform, string> = {
  anthropic: 'bg-amber-600 text-white hover:bg-amber-700 active:bg-amber-800 dark:bg-amber-500/80 dark:hover:bg-amber-500',
  openai: 'bg-emerald-600 text-white hover:bg-emerald-700 active:bg-emerald-800 dark:bg-emerald-600/80 dark:hover:bg-emerald-600',
  antigravity: 'bg-purple-500 text-white hover:bg-purple-600 active:bg-purple-700 dark:bg-purple-500/80 dark:hover:bg-purple-500',
  gemini: 'bg-blue-500 text-white hover:bg-blue-600 active:bg-blue-700 dark:bg-blue-500/80 dark:hover:bg-blue-500',
  grok: 'bg-zinc-800 text-white hover:bg-zinc-900 active:bg-black dark:bg-zinc-700 dark:hover:bg-zinc-600',
  deepseek: 'bg-indigo-600 text-white hover:bg-indigo-700 active:bg-indigo-800 dark:bg-indigo-500 dark:hover:bg-indigo-400',
  qwen: 'bg-violet-600 text-white hover:bg-violet-700 active:bg-violet-800 dark:bg-violet-500 dark:hover:bg-violet-400',
  glm: 'bg-cyan-700 text-white hover:bg-cyan-800 active:bg-cyan-900 dark:bg-cyan-600 dark:hover:bg-cyan-500',
  kimi: 'bg-teal-700 text-white hover:bg-teal-800 active:bg-teal-900 dark:bg-teal-600 dark:hover:bg-teal-500',
  doubao: 'bg-sky-700 text-white hover:bg-sky-800 active:bg-sky-900 dark:bg-sky-600 dark:hover:bg-sky-500',
  minimax: 'bg-rose-600 text-white hover:bg-rose-700 active:bg-rose-800 dark:bg-rose-500 dark:hover:bg-rose-400',
  mimo: 'bg-orange-600 text-white hover:bg-orange-700 active:bg-orange-800 dark:bg-orange-500 dark:hover:bg-orange-400',
  composite: 'bg-lime-700 text-white hover:bg-lime-800 active:bg-lime-900 dark:bg-lime-600 dark:hover:bg-lime-500',
}
const BUTTON_DEFAULT = 'bg-primary-500 text-white hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500'

// ── Discount badge ──────────────────────────────────────────────────
const DISCOUNT: Record<Platform, string> = {
  anthropic: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  grok: 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-200',
  deepseek: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/40 dark:text-indigo-300',
  qwen: 'bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-300',
  glm: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300',
  kimi: 'bg-teal-100 text-teal-800 dark:bg-teal-900/40 dark:text-teal-300',
  doubao: 'bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-300',
  minimax: 'bg-rose-100 text-rose-800 dark:bg-rose-900/40 dark:text-rose-300',
  mimo: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300',
  composite: 'bg-lime-100 text-lime-800 dark:bg-lime-900/40 dark:text-lime-300',
}
const DISCOUNT_DEFAULT = 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'

// ── Header gradient (subscription confirm) ─────────────────────────
const GRADIENT: Record<Platform, string> = {
  anthropic: 'from-amber-500 to-amber-700',
  openai: 'from-emerald-500 to-emerald-600',
  antigravity: 'from-purple-500 to-purple-600',
  gemini: 'from-blue-500 to-blue-600',
  grok: 'from-zinc-700 to-zinc-900',
  deepseek: 'from-indigo-600 to-cyan-600',
  qwen: 'from-violet-600 to-indigo-600',
  glm: 'from-cyan-600 to-cyan-800',
  kimi: 'from-teal-600 to-teal-800',
  doubao: 'from-sky-600 to-sky-800',
  minimax: 'from-rose-600 to-orange-600',
  mimo: 'from-orange-500 to-orange-700',
  composite: 'from-lime-600 to-lime-800',
}
const GRADIENT_DEFAULT = 'from-primary-500 to-primary-600'

// ── Header text (light text on gradient bg) ────────────────────────
const GRADIENT_TEXT: Record<Platform, string> = {
  anthropic: 'text-amber-100',
  openai: 'text-emerald-100',
  antigravity: 'text-purple-100',
  gemini: 'text-blue-100',
  grok: 'text-zinc-100',
  deepseek: 'text-indigo-100',
  qwen: 'text-violet-100',
  glm: 'text-cyan-100',
  kimi: 'text-teal-100',
  doubao: 'text-sky-100',
  minimax: 'text-rose-100',
  mimo: 'text-orange-100',
  composite: 'text-lime-100',
}
const GRADIENT_TEXT_DEFAULT = 'text-primary-100'

const GRADIENT_SUBTEXT: Record<Platform, string> = {
  anthropic: 'text-amber-200',
  openai: 'text-emerald-200',
  antigravity: 'text-purple-200',
  gemini: 'text-blue-200',
  grok: 'text-zinc-300',
  deepseek: 'text-indigo-200',
  qwen: 'text-violet-200',
  glm: 'text-cyan-200',
  kimi: 'text-teal-200',
  doubao: 'text-sky-200',
  minimax: 'text-rose-200',
  mimo: 'text-orange-200',
  composite: 'text-lime-200',
}
const GRADIENT_SUBTEXT_DEFAULT = 'text-primary-200'

// ── Public API ──────────────────────────────────────────────────────

function isPlatform(p: string): p is Platform {
  return p === 'anthropic' || p === 'openai' || p === 'antigravity' || p === 'gemini' ||
    p === 'grok' || p === 'deepseek' || p === 'qwen' || p === 'glm' || p === 'kimi' ||
    p === 'doubao' || p === 'minimax' ||
    p === 'mimo' || p === 'composite'
}

export function platformBadgeClass(p: string): string {
  return isPlatform(p) ? BADGE[p] : BADGE_DEFAULT
}

export function platformBadgeLightClass(p: string): string {
  return isPlatform(p) ? BADGE_LIGHT[p] : BADGE_DEFAULT
}

export function platformBorderClass(p: string): string {
  return isPlatform(p) ? BORDER[p] : BORDER_DEFAULT
}

export function platformBorderStrongClass(p: string): string {
  return isPlatform(p) ? BORDER_STRONG[p] : BORDER_STRONG_DEFAULT
}

export function platformAccentColor(p: string): string {
  return isPlatform(p) ? ACCENT[p] : ACCENT_DEFAULT
}

export function platformAccentBarClass(p: string): string {
  return isPlatform(p) ? ACCENT_BAR[p] : ACCENT_BAR_DEFAULT
}

export function platformAccentDotClass(p: string): string {
  return isPlatform(p) ? ACCENT_DOT[p] : ACCENT_DOT_DEFAULT
}

export function platformTextClass(p: string): string {
  return isPlatform(p) ? TEXT[p] : TEXT_DEFAULT
}

export function platformIconClass(p: string): string {
  return isPlatform(p) ? ICON[p] : ICON_DEFAULT
}

export function platformAccentHex(p: string): string {
  return isPlatform(p) ? ACCENT_HEX[p] : ACCENT_HEX_DEFAULT
}

export function platformButtonClass(p: string): string {
  return isPlatform(p) ? BUTTON[p] : BUTTON_DEFAULT
}

export function platformDiscountClass(p: string): string {
  return isPlatform(p) ? DISCOUNT[p] : DISCOUNT_DEFAULT
}

export function platformGradientClass(p: string): string {
  return isPlatform(p) ? GRADIENT[p] : GRADIENT_DEFAULT
}

export function platformGradientTextClass(p: string): string {
  return isPlatform(p) ? GRADIENT_TEXT[p] : GRADIENT_TEXT_DEFAULT
}

export function platformGradientSubtextClass(p: string): string {
  return isPlatform(p) ? GRADIENT_SUBTEXT[p] : GRADIENT_SUBTEXT_DEFAULT
}

export function platformLabel(p: string): string {
  switch (p) {
    case 'anthropic': return 'Anthropic'
    case 'openai': return 'OpenAI'
    case 'antigravity': return 'Antigravity'
    case 'gemini': return 'Gemini'
    case 'grok': return 'Grok'
    case 'deepseek': return 'DeepSeek'
    case 'qwen': return 'Qwen'
    case 'glm': return '智谱AI'
    case 'kimi': return 'Kimi'
    case 'doubao': return 'ByteDance'
    case 'minimax': return 'MiniMax'
    case 'mimo': return 'MiMo'
    case 'composite': return 'Composite'
    default: return p || 'API'
  }
}
