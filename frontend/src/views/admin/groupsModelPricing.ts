import type { ChannelModelPricing } from "@/api/admin/channels";
import type { PricingFormEntry } from "@/components/admin/channel/types";
import {
  apiIntervalsToForm,
  findPricingModelConflict,
  formIntervalsToAPI,
  isMissingRequiredUnitPrice,
  mTokToPerToken,
  perTokenToMTok,
  toNullableNumber,
  validateIntervals,
  validatePricingEntryPrices,
} from "@/components/admin/channel/types";
import type { AccountPlatform, GroupPlatform } from "@/types";
import { CONCRETE_PLATFORM_ORDER } from "@/utils/providerPresets";

const DEFAULT_COMPOSITE_PRICING_PLATFORM = CONCRETE_PLATFORM_ORDER[0];
const concretePlatforms = new Set<string>(CONCRETE_PLATFORM_ORDER);

type TranslateFn = (
  key: string,
  params?: Record<string, unknown>,
) => string;

export type GroupPricingValidationError =
  | { code: "modelsRequired"; entryIndex: number }
  | { code: "modelConflict"; model1: string; model2: string }
  | { code: "unitPriceRequired"; entryIndex: number; models: string[] }
  | {
      code: "invalidPrices";
      entryIndex: number;
      models: string[];
      detail: string;
    }
  | {
      code: "invalidIntervals";
      entryIndex: number;
      models: string[];
      detail: string;
    };

export function resolveGroupPricingPlatform(
  entry: Pick<PricingFormEntry, "platform">,
  groupPlatform: GroupPlatform,
): AccountPlatform {
  if (groupPlatform !== "composite") return groupPlatform;
  if (entry.platform && concretePlatforms.has(entry.platform)) {
    return entry.platform as AccountPlatform;
  }
  return DEFAULT_COMPOSITE_PRICING_PLATFORM;
}

export function createGroupPricingEntry(
  groupPlatform: GroupPlatform,
): PricingFormEntry {
  return {
    platform:
      groupPlatform === "composite"
        ? DEFAULT_COMPOSITE_PRICING_PLATFORM
        : groupPlatform,
    models: [],
    billing_mode: "token",
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    user_visible: true,
    intervals: [],
  };
}

export function validateGroupPricingEntries(
  pricing: PricingFormEntry[],
  t: TranslateFn,
): GroupPricingValidationError | null {
  for (let index = 0; index < pricing.length; index += 1) {
    if (pricing[index].models.length === 0) {
      return { code: "modelsRequired", entryIndex: index + 1 };
    }
  }

  const conflict = findPricingModelConflict(
    pricing.flatMap((entry) => entry.models),
  );
  if (conflict) {
    return {
      code: "modelConflict",
      model1: conflict[0],
      model2: conflict[1],
    };
  }

  for (let index = 0; index < pricing.length; index += 1) {
    const entry = pricing[index];
    const priceDetail = validatePricingEntryPrices(entry, t);
    if (priceDetail) {
      return {
        code: "invalidPrices",
        entryIndex: index + 1,
        models: entry.models,
        detail: priceDetail,
      };
    }
    if (isMissingRequiredUnitPrice(entry)) {
      return {
        code: "unitPriceRequired",
        entryIndex: index + 1,
        models: entry.models,
      };
    }

    const intervals = entry.billing_mode === "token" ? [] : entry.intervals;
    const detail = validateIntervals(intervals, entry.billing_mode, t);
    if (detail) {
      return {
        code: "invalidIntervals",
        entryIndex: index + 1,
        models: entry.models,
        detail,
      };
    }
  }

  return null;
}

export function groupPricingFromAPI(
  pricing: ChannelModelPricing[] | undefined,
  groupPlatform: GroupPlatform,
): PricingFormEntry[] {
  return (pricing || []).map((entry) => ({
    platform: resolveGroupPricingPlatform(entry, groupPlatform),
    models: entry.models || [],
    billing_mode: entry.billing_mode || "token",
    input_price: perTokenToMTok(entry.input_price),
    output_price: perTokenToMTok(entry.output_price),
    cache_write_price: perTokenToMTok(entry.cache_write_price),
    cache_read_price: perTokenToMTok(entry.cache_read_price),
    image_input_price: perTokenToMTok(entry.image_input_price),
    image_output_price: perTokenToMTok(entry.image_output_price),
    per_request_price: entry.per_request_price,
    user_visible: true,
    intervals: apiIntervalsToForm(entry.intervals || []),
  }));
}

export function groupPricingToAPI(
  pricing: PricingFormEntry[],
  groupPlatform: GroupPlatform,
): ChannelModelPricing[] {
  return pricing
    .filter((entry) => entry.models.length > 0)
    .map((entry) => ({
      platform: resolveGroupPricingPlatform(entry, groupPlatform),
      models: entry.models,
      billing_mode: entry.billing_mode,
      input_price: mTokToPerToken(entry.input_price),
      output_price: mTokToPerToken(entry.output_price),
      cache_write_price: mTokToPerToken(entry.cache_write_price),
      cache_read_price: mTokToPerToken(entry.cache_read_price),
      image_input_price: mTokToPerToken(entry.image_input_price),
      image_output_price: mTokToPerToken(entry.image_output_price),
      per_request_price: toNullableNumber(entry.per_request_price),
      intervals:
        entry.billing_mode === "token"
          ? []
          : formIntervalsToAPI(entry.intervals || []),
    }));
}
