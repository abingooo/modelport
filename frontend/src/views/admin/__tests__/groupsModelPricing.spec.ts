import { describe, expect, it } from "vitest";

import type { ChannelModelPricing } from "@/api/admin/channels";
import type {
  IntervalFormEntry,
  PricingFormEntry,
} from "@/components/admin/channel/types";
import {
  createGroupPricingEntry,
  groupPricingFromAPI,
  groupPricingToAPI,
  resolveGroupPricingPlatform,
  validateGroupPricingEntries,
} from "../groupsModelPricing";

function apiPricing(
  overrides: Partial<ChannelModelPricing> = {},
): ChannelModelPricing {
  return {
    platform: "openai",
    models: ["gpt-5"],
    billing_mode: "token",
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    ...overrides,
  };
}

function formPricing(
  overrides: Partial<PricingFormEntry> = {},
): PricingFormEntry {
  return {
    ...createGroupPricingEntry("openai"),
    models: ["gpt-5"],
    ...overrides,
  };
}

function interval(
  overrides: Partial<IntervalFormEntry> = {},
): IntervalFormEntry {
  return {
    min_tokens: 0,
    max_tokens: null,
    tier_label: "",
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    per_request_price: null,
    sort_order: 0,
    ...overrides,
  };
}

const t = (key: string, params?: Record<string, unknown>) =>
  `${key}${params ? ` ${JSON.stringify(params)}` : ""}`;

describe("group model pricing conversion", () => {
  it("round-trips each concrete platform for a composite group", () => {
    const form = groupPricingFromAPI(
      [
        apiPricing({ platform: "anthropic", models: ["claude-*"] }),
        apiPricing({ platform: "openai", models: ["gpt-*"] }),
      ],
      "composite",
    );

    expect(form.map((entry) => entry.platform)).toEqual([
      "anthropic",
      "openai",
    ]);
    expect(
      groupPricingToAPI(form, "composite").map((entry) => entry.platform),
    ).toEqual(["anthropic", "openai"]);
  });

  it("forces a regular group entry to the group platform", () => {
    const entry = createGroupPricingEntry("openai");
    entry.platform = "anthropic";
    entry.models = ["gpt-5"];

    expect(resolveGroupPricingPlatform(entry, "openai")).toBe("openai");
    expect(groupPricingToAPI([entry], "openai")[0].platform).toBe("openai");
  });

  it("preserves partial token overrides without filling missing prices", () => {
    const form = groupPricingFromAPI(
      [apiPricing({ output_price: 0.00002 })],
      "openai",
    );

    expect(form[0]).toMatchObject({
      input_price: null,
      output_price: 20,
      cache_write_price: null,
      cache_read_price: null,
    });
    expect(groupPricingToAPI(form, "openai")[0]).toMatchObject({
      input_price: null,
      output_price: 0.00002,
      cache_write_price: null,
      cache_read_price: null,
    });
  });

  it("normalizes an invalid composite platform to a concrete platform", () => {
    const form = groupPricingFromAPI(
      [apiPricing({ platform: "composite" })],
      "composite",
    );

    expect(form[0].platform).toBe("anthropic");
    expect(groupPricingToAPI(form, "composite")[0].platform).toBe("anthropic");
  });
});

describe("group model pricing validation", () => {
  it("requires every pricing entry to include a model", () => {
    expect(validateGroupPricingEntries([
      formPricing({ models: [] }),
    ], t)).toMatchObject({ code: "modelsRequired", entryIndex: 1 });
  });

  it("rejects exact and wildcard pattern conflicts across composite platforms", () => {
    expect(validateGroupPricingEntries([
      formPricing({ platform: "openai", models: ["gpt-*"] }),
      formPricing({ platform: "anthropic", models: ["GPT-5"] }),
    ], t)).toMatchObject({
      code: "modelConflict",
      model1: "gpt-*",
      model2: "GPT-5",
    });
  });

  it("requires video pricing but accepts an explicit zero fallback", () => {
    const video = formPricing({
      billing_mode: "video",
      per_request_price: null,
      intervals: [],
    });

    expect(validateGroupPricingEntries([video], t)).toMatchObject({
      code: "unitPriceRequired",
    });
    expect(validateGroupPricingEntries([
      { ...video, per_request_price: 0 },
    ], t)).toBeNull();
  });

  it("rejects negative and non-finite top-level prices", () => {
    expect(validateGroupPricingEntries([
      formPricing({ input_price: -1 }),
    ], t)).toMatchObject({ code: "invalidPrices" });
    expect(validateGroupPricingEntries([
      formPricing({ output_price: Number.NaN }),
    ], t)).toMatchObject({ code: "invalidPrices" });
  });

  it("rejects invalid interval ranges", () => {
    expect(validateGroupPricingEntries([
      formPricing({
        billing_mode: "image",
        intervals: [interval({
          tier_label: "1K",
          min_tokens: 10,
          max_tokens: 5,
          per_request_price: 0.1,
        })],
      }),
    ], t)).toMatchObject({
      code: "invalidIntervals",
      detail: expect.stringContaining("maxGreaterThanMin"),
    });
  });

  it("rejects duplicate tier labels case-insensitively", () => {
    expect(validateGroupPricingEntries([
      formPricing({
        billing_mode: "video",
        intervals: [
          interval({ tier_label: "1080p", per_request_price: 0.2 }),
          interval({ tier_label: " 1080P ", per_request_price: 0.3 }),
        ],
      }),
    ], t)).toMatchObject({
      code: "invalidIntervals",
      detail: expect.stringContaining("duplicateTier"),
    });
  });

  it("rejects a tier with no price fields", () => {
    expect(validateGroupPricingEntries([
      formPricing({
        billing_mode: "per_request",
        intervals: [interval({ tier_label: "realtime" })],
      }),
    ], t)).toMatchObject({
      code: "invalidIntervals",
      detail: expect.stringContaining("missingPrice"),
    });
  });
});
