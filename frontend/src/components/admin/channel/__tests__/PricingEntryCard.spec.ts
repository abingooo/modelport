import { flushPromises, shallowMount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import type { PricingFormEntry } from "../types";
import ModelTagInput from "../ModelTagInput.vue";
import PricingEntryCard from "../PricingEntryCard.vue";
import Select from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";

const { getModelDefaultPricing } = vi.hoisted(() => ({
  getModelDefaultPricing: vi.fn(),
}));

vi.mock("@/api/admin/channels", () => ({
  default: { getModelDefaultPricing },
}));

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  };
});

function pricingEntry(
  overrides: Partial<PricingFormEntry> = {},
): PricingFormEntry {
  return {
    platform: "anthropic",
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
    ...overrides,
  };
}

describe("PricingEntryCard group pricing options", () => {
  it("selects a concrete platform while hiding channel-only controls", async () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: {
        entry: pricingEntry(),
        platform: "anthropic",
        platformOptions: [
          { value: "anthropic", label: "Anthropic" },
          { value: "openai", label: "OpenAI" },
        ],
        showPlazaVisibility: false,
        autoFillDefaultPricing: false,
      },
    });

    expect(wrapper.findComponent(Toggle).exists()).toBe(false);
    const selects = wrapper.findAllComponents(Select);
    expect(selects).toHaveLength(2);

    selects[0].vm.$emit("update:modelValue", "openai");
    await wrapper
      .findComponent(ModelTagInput)
      .vm.$emit("update:models", ["gpt-5"]);
    await flushPromises();

    expect(wrapper.emitted("update")).toContainEqual([
      expect.objectContaining({ platform: "openai" }),
    ]);
    expect(wrapper.emitted("update")).toContainEqual([
      expect.objectContaining({ models: ["gpt-5"] }),
    ]);
    expect(getModelDefaultPricing).not.toHaveBeenCalled();
  });

  it("keeps image and video presets plus the custom tier action", async () => {
    const wrapper = shallowMount(PricingEntryCard, {
      props: { entry: pricingEntry({ billing_mode: "image" }) },
    });

    expect(wrapper.text()).toContain("1K");
    expect(wrapper.text()).toContain("2K");
    expect(wrapper.text()).toContain("4K");
    expect(wrapper.text()).toContain("admin.channels.form.customTier");

    await wrapper.setProps({ entry: pricingEntry({ billing_mode: "video" }) });
    expect(wrapper.text()).toContain("480p");
    expect(wrapper.text()).toContain("720p");
    expect(wrapper.text()).toContain("1080p");
    expect(wrapper.text()).toContain("admin.channels.form.customTier");
  });
});
