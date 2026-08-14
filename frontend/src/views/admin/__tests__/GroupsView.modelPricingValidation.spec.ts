import { defineComponent } from "vue";
import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { AdminGroup } from "@/types";
import GroupsView from "../GroupsView.vue";

const {
  createGroup,
  getCapacitySummary,
  getLiveCapability,
  getModelsListCandidates,
  getUsageSummary,
  listGroups,
  showError,
  updateGroup,
} = vi.hoisted(() => ({
  createGroup: vi.fn(),
  getCapacitySummary: vi.fn(),
  getLiveCapability: vi.fn(),
  getModelsListCandidates: vi.fn(),
  getUsageSummary: vi.fn(),
  listGroups: vi.fn(),
  showError: vi.fn(),
  updateGroup: vi.fn(),
}));

vi.mock("@/api/admin", () => ({
  adminAPI: {
    groups: {
      list: listGroups,
      create: createGroup,
      update: updateGroup,
      getModelsListCandidates,
      getUsageSummary,
      getCapacitySummary,
      getLiveCapability,
    },
    accounts: {
      list: vi.fn(),
      getById: vi.fn(),
    },
  },
}));

vi.mock("@/stores/app", () => ({
  useAppStore: () => ({ showError, showSuccess: vi.fn() }),
}));

vi.mock("@/stores/onboarding", () => ({
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn(),
  }),
}));

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  };
});

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
});

const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] } },
  template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div></div>',
});

const groupWithMissingVideoPrice = {
  id: 42,
  name: "Video group",
  description: null,
  platform: "openai",
  rate_multiplier: 1,
  rpm_limit: 0,
  is_free: false,
  is_exclusive: false,
  status: "active",
  subscription_type: "standard",
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  long_context_pricing_enabled: true,
  model_pricing: [{
    platform: "openai",
    models: ["sora-2"],
    billing_mode: "video",
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
  }],
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  search_price_per_1k: null,
  audio_realtime_price_per_min: null,
  audio_tts_price_per_million_chars: null,
  audio_stt_price_per_hour: null,
  peak_rate_enabled: false,
  peak_start: "",
  peak_end: "",
  peak_rate_multiplier: 1,
  profit_control_enabled: false,
  profit_min_margin: 0,
  profit_safety_buffer: 0,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_live: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: "2026-08-14T00:00:00Z",
  updated_at: "2026-08-14T00:00:00Z",
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  supported_model_scopes: [],
  account_count: 0,
  active_account_count: 0,
  rate_limited_account_count: 0,
  sort_order: 0,
} as AdminGroup;

function mountView() {
  return mount(GroupsView, {
    global: {
      stubs: {
        AppLayout: { template: "<main><slot /></main>" },
        TablePageLayout: {
          template: '<section><slot name="filters" /><slot name="table" /><slot name="pagination" /></section>',
        },
        DataTable: DataTableStub,
        Pagination: true,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        EmptyState: true,
        Select: true,
        PlatformIcon: true,
        Icon: true,
        GroupBadge: true,
        GroupCapacityBadge: true,
        GroupRateMultipliersModal: true,
        GroupRPMOverridesModal: true,
        ReasoningEffortPolicyFields: true,
        PricingEntryCard: true,
        VueDraggable: { template: "<div><slot /></div>" },
      },
    },
  });
}

describe("GroupsView model pricing submission validation", () => {
  beforeEach(() => {
    localStorage.clear();
    for (const fn of [
      createGroup,
      getCapacitySummary,
      getLiveCapability,
      getModelsListCandidates,
      getUsageSummary,
      listGroups,
      showError,
      updateGroup,
    ]) {
      fn.mockReset();
    }
    listGroups.mockResolvedValue({
      items: [groupWithMissingVideoPrice],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    });
    getCapacitySummary.mockResolvedValue([]);
    getLiveCapability.mockResolvedValue({ supported: false });
    getModelsListCandidates.mockResolvedValue([]);
    getUsageSummary.mockResolvedValue([]);
  });

  afterEach(() => {
    localStorage.clear();
  });

  it("blocks create before the API call when a pricing entry has no models", async () => {
    const wrapper = mountView();
    await flushPromises();

    await wrapper.get('[data-tour="groups-create-btn"]').trigger("click");
    await wrapper.get('[data-tour="group-form-name"]').setValue("New group");
    const addPricing = wrapper
      .findAll("button")
      .find((button) => button.text() === "admin.groups.modelPricing.add");
    expect(addPricing).toBeTruthy();
    await addPricing!.trigger("click");
    await wrapper.get("#create-group-form").trigger("submit");

    expect(showError).toHaveBeenCalledWith(
      "admin.groups.modelPricing.modelsRequired",
    );
    expect(createGroup).not.toHaveBeenCalled();
    wrapper.unmount();
  });

  it("blocks update before the API call when video pricing is missing", async () => {
    const wrapper = mountView();
    await flushPromises();

    const edit = wrapper
      .findAll("button")
      .find((button) => button.text().includes("common.edit"));
    expect(edit).toBeTruthy();
    await edit!.trigger("click");
    await flushPromises();
    await wrapper.get("#edit-group-form").trigger("submit");

    expect(showError).toHaveBeenCalledWith(
      "admin.groups.modelPricing.unitPriceRequired",
    );
    expect(updateGroup).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});
