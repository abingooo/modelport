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
  | 'siliconflow'
  | 'openrouter'
  | 'minimax'
  | 'mimo'
  | 'composite'

// ── Badge (bg + text + border, for inline badges with border) ───────
const BADGE: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 border-orange-500/30 dark:text-orange-400',
  openai: 'bg-green-500/10 text-green-600 border-green-500/30 dark:text-green-400',
  antigravity: 'bg-purple-500/10 text-purple-600 border-purple-500/30 dark:text-purple-400',
  gemini: 'bg-blue-500/10 text-blue-600 border-blue-500/30 dark:text-blue-400',
  grok: 'bg-zinc-800/10 text-zinc-800 border-zinc-800/30 dark:bg-zinc-500/10 dark:text-zinc-200 dark:border-zinc-500/30',
  deepseek: 'bg-indigo-500/10 text-indigo-700 border-indigo-500/30 dark:text-indigo-300',
  qwen: 'bg-cyan-500/10 text-cyan-700 border-cyan-500/30 dark:text-cyan-300',
  glm: 'bg-sky-500/10 text-sky-700 border-sky-500/30 dark:text-sky-300',
  kimi: 'bg-pink-500/10 text-pink-700 border-pink-500/30 dark:text-pink-300',
  doubao: 'bg-red-500/10 text-red-700 border-red-500/30 dark:text-red-300',
  siliconflow: 'bg-teal-500/10 text-teal-700 border-teal-500/30 dark:text-teal-300',
  openrouter: 'bg-amber-500/10 text-amber-700 border-amber-500/30 dark:text-amber-300',
  minimax: 'bg-violet-500/10 text-violet-700 border-violet-500/30 dark:text-violet-300',
  mimo: 'bg-lime-500/10 text-lime-700 border-lime-500/30 dark:text-lime-300',
  composite: 'bg-cyan-500/10 text-cyan-700 border-cyan-500/30 dark:text-cyan-300',
}
const BADGE_DEFAULT = 'bg-slate-500/10 text-slate-600 border-slate-500/30 dark:text-slate-400'

// ── Light badge (softer bg, no border) ──────────────────────────────
const BADGE_LIGHT: Record<Platform, string> = {
  anthropic: 'bg-orange-500/10 text-orange-600 dark:bg-orange-500/10 dark:text-orange-300',
  openai: 'bg-green-500/10 text-green-600 dark:bg-green-500/10 dark:text-green-300',
  antigravity: 'bg-purple-500/10 text-purple-600 dark:bg-purple-500/10 dark:text-purple-300',
  gemini: 'bg-blue-500/10 text-blue-600 dark:bg-blue-500/10 dark:text-blue-300',
  grok: 'bg-zinc-800/10 text-zinc-800 dark:bg-zinc-500/10 dark:text-zinc-200',
  deepseek: 'bg-indigo-500/10 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300',
  qwen: 'bg-cyan-500/10 text-cyan-700 dark:bg-cyan-500/10 dark:text-cyan-300',
  glm: 'bg-sky-500/10 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300',
  kimi: 'bg-pink-500/10 text-pink-700 dark:bg-pink-500/10 dark:text-pink-300',
  doubao: 'bg-red-500/10 text-red-700 dark:bg-red-500/10 dark:text-red-300',
  siliconflow: 'bg-teal-500/10 text-teal-700 dark:bg-teal-500/10 dark:text-teal-300',
  openrouter: 'bg-amber-500/10 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300',
  minimax: 'bg-violet-500/10 text-violet-700 dark:bg-violet-500/10 dark:text-violet-300',
  mimo: 'bg-lime-500/10 text-lime-700 dark:bg-lime-500/10 dark:text-lime-300',
  composite: 'bg-cyan-500/10 text-cyan-700 dark:bg-cyan-500/10 dark:text-cyan-300',
}

// ── Border ──────────────────────────────────────────────────────────
const BORDER: Record<Platform, string> = {
  anthropic: 'border-orange-500/20 dark:border-orange-500/20',
  openai: 'border-green-500/20 dark:border-green-500/20',
  antigravity: 'border-purple-500/20 dark:border-purple-500/20',
  gemini: 'border-blue-500/20 dark:border-blue-500/20',
  grok: 'border-zinc-800/20 dark:border-zinc-500/20',
  deepseek: 'border-indigo-500/20 dark:border-indigo-500/20',
  qwen: 'border-cyan-500/20 dark:border-cyan-500/20',
  glm: 'border-sky-500/20 dark:border-sky-500/20',
  kimi: 'border-pink-500/20 dark:border-pink-500/20',
  doubao: 'border-red-500/20 dark:border-red-500/20',
  siliconflow: 'border-teal-500/20 dark:border-teal-500/20',
  openrouter: 'border-amber-500/20 dark:border-amber-500/20',
  minimax: 'border-violet-500/20 dark:border-violet-500/20',
  mimo: 'border-lime-500/20 dark:border-lime-500/20',
  composite: 'border-cyan-500/20 dark:border-cyan-500/20',
}
const BORDER_DEFAULT = 'border-gray-200 dark:border-dark-700'

// ── Accent bar (gradient) ───────────────────────────────────────────
const ACCENT_BAR: Record<Platform, string> = {
  anthropic: 'bg-gradient-to-r from-orange-400 to-orange-500',
  openai: 'bg-gradient-to-r from-emerald-400 to-emerald-500',
  antigravity: 'bg-gradient-to-r from-purple-400 to-purple-500',
  gemini: 'bg-gradient-to-r from-blue-400 to-blue-500',
  grok: 'bg-gradient-to-r from-zinc-700 to-zinc-900',
  deepseek: 'bg-gradient-to-r from-indigo-500 to-cyan-500',
  qwen: 'bg-gradient-to-r from-cyan-500 to-sky-500',
  glm: 'bg-gradient-to-r from-sky-500 to-blue-500',
  kimi: 'bg-gradient-to-r from-pink-500 to-rose-500',
  doubao: 'bg-gradient-to-r from-red-500 to-rose-600',
  siliconflow: 'bg-gradient-to-r from-teal-500 to-emerald-500',
  openrouter: 'bg-gradient-to-r from-amber-500 to-yellow-500',
  minimax: 'bg-gradient-to-r from-violet-500 to-fuchsia-500',
  mimo: 'bg-gradient-to-r from-lime-500 to-green-500',
  composite: 'bg-gradient-to-r from-slate-500 to-cyan-500',
}
const ACCENT_BAR_DEFAULT = 'bg-gradient-to-r from-primary-400 to-primary-500'

// ── Text (price, icon) ─────────────────────────────────────────────
const TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-600 dark:text-orange-400',
  openai: 'text-emerald-600 dark:text-emerald-400',
  antigravity: 'text-purple-600 dark:text-purple-400',
  gemini: 'text-blue-600 dark:text-blue-400',
  grok: 'text-zinc-800 dark:text-zinc-200',
  deepseek: 'text-indigo-700 dark:text-indigo-300',
  qwen: 'text-cyan-700 dark:text-cyan-300',
  glm: 'text-sky-700 dark:text-sky-300',
  kimi: 'text-pink-700 dark:text-pink-300',
  doubao: 'text-red-700 dark:text-red-300',
  siliconflow: 'text-teal-700 dark:text-teal-300',
  openrouter: 'text-amber-700 dark:text-amber-300',
  minimax: 'text-violet-700 dark:text-violet-300',
  mimo: 'text-lime-700 dark:text-lime-300',
  composite: 'text-cyan-700 dark:text-cyan-300',
}
const TEXT_DEFAULT = 'text-primary-600 dark:text-primary-400'

// ── Icon (check mark etc.) ──────────────────────────────────────────
const ICON: Record<Platform, string> = {
  anthropic: 'text-orange-500 dark:text-orange-400',
  openai: 'text-emerald-500 dark:text-emerald-400',
  antigravity: 'text-purple-500 dark:text-purple-400',
  gemini: 'text-blue-500 dark:text-blue-400',
  grok: 'text-zinc-800 dark:text-zinc-200',
  deepseek: 'text-indigo-600 dark:text-indigo-300',
  qwen: 'text-cyan-600 dark:text-cyan-300',
  glm: 'text-sky-600 dark:text-sky-300',
  kimi: 'text-pink-600 dark:text-pink-300',
  doubao: 'text-red-600 dark:text-red-300',
  siliconflow: 'text-teal-600 dark:text-teal-300',
  openrouter: 'text-amber-600 dark:text-amber-300',
  minimax: 'text-violet-600 dark:text-violet-300',
  mimo: 'text-lime-600 dark:text-lime-300',
  composite: 'text-cyan-600 dark:text-cyan-300',
}
const ICON_DEFAULT = 'text-primary-500 dark:text-primary-400'

// ── Button (solid bg) ───────────────────────────────────────────────
const BUTTON: Record<Platform, string> = {
  anthropic: 'bg-orange-500 text-white hover:bg-orange-600 active:bg-orange-700 dark:bg-orange-500/80 dark:hover:bg-orange-500',
  openai: 'bg-green-600 text-white hover:bg-green-700 active:bg-green-800 dark:bg-green-600/80 dark:hover:bg-green-600',
  antigravity: 'bg-purple-500 text-white hover:bg-purple-600 active:bg-purple-700 dark:bg-purple-500/80 dark:hover:bg-purple-500',
  gemini: 'bg-blue-500 text-white hover:bg-blue-600 active:bg-blue-700 dark:bg-blue-500/80 dark:hover:bg-blue-500',
  grok: 'bg-zinc-800 text-white hover:bg-zinc-900 active:bg-black dark:bg-zinc-700 dark:hover:bg-zinc-600',
  deepseek: 'bg-indigo-600 text-white hover:bg-indigo-700 active:bg-indigo-800 dark:bg-indigo-500 dark:hover:bg-indigo-400',
  qwen: 'bg-cyan-600 text-white hover:bg-cyan-700 active:bg-cyan-800 dark:bg-cyan-500 dark:hover:bg-cyan-400',
  glm: 'bg-sky-600 text-white hover:bg-sky-700 active:bg-sky-800 dark:bg-sky-500 dark:hover:bg-sky-400',
  kimi: 'bg-pink-600 text-white hover:bg-pink-700 active:bg-pink-800 dark:bg-pink-500 dark:hover:bg-pink-400',
  doubao: 'bg-red-600 text-white hover:bg-red-700 active:bg-red-800 dark:bg-red-500 dark:hover:bg-red-400',
  siliconflow: 'bg-teal-600 text-white hover:bg-teal-700 active:bg-teal-800 dark:bg-teal-500 dark:hover:bg-teal-400',
  openrouter: 'bg-amber-600 text-white hover:bg-amber-700 active:bg-amber-800 dark:bg-amber-500 dark:hover:bg-amber-400',
  minimax: 'bg-violet-600 text-white hover:bg-violet-700 active:bg-violet-800 dark:bg-violet-500 dark:hover:bg-violet-400',
  mimo: 'bg-lime-600 text-white hover:bg-lime-700 active:bg-lime-800 dark:bg-lime-500 dark:hover:bg-lime-400',
  composite: 'bg-cyan-700 text-white hover:bg-cyan-800 active:bg-cyan-900 dark:bg-cyan-600 dark:hover:bg-cyan-500',
}
const BUTTON_DEFAULT = 'bg-primary-500 text-white hover:bg-primary-600 dark:bg-primary-600 dark:hover:bg-primary-500'

// ── Discount badge ──────────────────────────────────────────────────
const DISCOUNT: Record<Platform, string> = {
  anthropic: 'bg-orange-100 text-orange-700 dark:bg-orange-900/40 dark:text-orange-300',
  openai: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300',
  antigravity: 'bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300',
  gemini: 'bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300',
  grok: 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-200',
  deepseek: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-900/40 dark:text-indigo-300',
  qwen: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300',
  glm: 'bg-sky-100 text-sky-800 dark:bg-sky-900/40 dark:text-sky-300',
  kimi: 'bg-pink-100 text-pink-800 dark:bg-pink-900/40 dark:text-pink-300',
  doubao: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  siliconflow: 'bg-teal-100 text-teal-800 dark:bg-teal-900/40 dark:text-teal-300',
  openrouter: 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  minimax: 'bg-violet-100 text-violet-800 dark:bg-violet-900/40 dark:text-violet-300',
  mimo: 'bg-lime-100 text-lime-800 dark:bg-lime-900/40 dark:text-lime-300',
  composite: 'bg-cyan-100 text-cyan-800 dark:bg-cyan-900/40 dark:text-cyan-300',
}
const DISCOUNT_DEFAULT = 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'

// ── Header gradient (subscription confirm) ─────────────────────────
const GRADIENT: Record<Platform, string> = {
  anthropic: 'from-orange-500 to-orange-600',
  openai: 'from-emerald-500 to-emerald-600',
  antigravity: 'from-purple-500 to-purple-600',
  gemini: 'from-blue-500 to-blue-600',
  grok: 'from-zinc-700 to-zinc-900',
  deepseek: 'from-indigo-600 to-cyan-600',
  qwen: 'from-cyan-600 to-sky-600',
  glm: 'from-sky-600 to-blue-600',
  kimi: 'from-pink-600 to-rose-600',
  doubao: 'from-red-600 to-rose-700',
  siliconflow: 'from-teal-600 to-emerald-600',
  openrouter: 'from-amber-600 to-yellow-600',
  minimax: 'from-violet-600 to-fuchsia-600',
  mimo: 'from-lime-600 to-green-600',
  composite: 'from-slate-600 to-cyan-600',
}
const GRADIENT_DEFAULT = 'from-primary-500 to-primary-600'

// ── Header text (light text on gradient bg) ────────────────────────
const GRADIENT_TEXT: Record<Platform, string> = {
  anthropic: 'text-orange-100',
  openai: 'text-emerald-100',
  antigravity: 'text-purple-100',
  gemini: 'text-blue-100',
  grok: 'text-zinc-100',
  deepseek: 'text-indigo-100',
  qwen: 'text-cyan-100',
  glm: 'text-sky-100',
  kimi: 'text-pink-100',
  doubao: 'text-red-100',
  siliconflow: 'text-teal-100',
  openrouter: 'text-amber-100',
  minimax: 'text-violet-100',
  mimo: 'text-lime-100',
  composite: 'text-cyan-100',
}
const GRADIENT_TEXT_DEFAULT = 'text-primary-100'

const GRADIENT_SUBTEXT: Record<Platform, string> = {
  anthropic: 'text-orange-200',
  openai: 'text-emerald-200',
  antigravity: 'text-purple-200',
  gemini: 'text-blue-200',
  grok: 'text-zinc-300',
  deepseek: 'text-indigo-200',
  qwen: 'text-cyan-200',
  glm: 'text-sky-200',
  kimi: 'text-pink-200',
  doubao: 'text-red-200',
  siliconflow: 'text-teal-200',
  openrouter: 'text-amber-200',
  minimax: 'text-violet-200',
  mimo: 'text-lime-200',
  composite: 'text-cyan-200',
}
const GRADIENT_SUBTEXT_DEFAULT = 'text-primary-200'

// ── Public API ──────────────────────────────────────────────────────

function isPlatform(p: string): p is Platform {
  return p === 'anthropic' || p === 'openai' || p === 'antigravity' || p === 'gemini' ||
    p === 'grok' || p === 'deepseek' || p === 'qwen' || p === 'glm' || p === 'kimi' ||
    p === 'doubao' || p === 'siliconflow' || p === 'openrouter' || p === 'minimax' ||
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

export function platformAccentBarClass(p: string): string {
  return isPlatform(p) ? ACCENT_BAR[p] : ACCENT_BAR_DEFAULT
}

export function platformTextClass(p: string): string {
  return isPlatform(p) ? TEXT[p] : TEXT_DEFAULT
}

export function platformIconClass(p: string): string {
  return isPlatform(p) ? ICON[p] : ICON_DEFAULT
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
    case 'glm': return 'GLM'
    case 'kimi': return 'Kimi'
    case 'doubao': return 'ByteDance'
    case 'siliconflow': return 'SiliconFlow'
    case 'openrouter': return 'OpenRouter'
    case 'minimax': return 'MiniMax'
    case 'mimo': return 'MiMo'
    case 'composite': return 'Composite'
    default: return p || 'API'
  }
}
