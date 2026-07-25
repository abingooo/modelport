import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

const { copyToClipboardMock } = vi.hoisted(() => ({
  copyToClipboardMock: vi.fn().mockResolvedValue(true)
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard: copyToClipboardMock
  })
}))

import UseKeyModal from '../UseKeyModal.vue'

const mountDeepSeekModal = () => mount(UseKeyModal, {
  props: {
    show: true,
    apiKey: 'sk-modelport-deepseek-test',
    baseUrl: 'https://modelport.example/v1',
    platform: 'deepseek'
  },
  global: {
    stubs: {
      BaseDialog: {
        template: '<div><slot /><slot name="footer" /></div>'
      },
      Icon: {
        template: '<span />'
      }
    }
  }
})

describe('UseKeyModal', () => {
  it('renders Grok Build and OpenCode setup for Grok groups', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-grok-test',
        baseUrl: 'https://example.com/v1',
        platform: 'grok'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const grokTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.grokCli')
    )
    expect(grokTab).toBeDefined()

    const grokConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('[model."grok"]'))
    expect(grokConfig).toBeDefined()
    expect(grokConfig).toContain('model = "grok-4.5"')
    expect(grokConfig).toContain('base_url = "https://example.com/v1"')
    expect(grokConfig).toContain('api_key = "sk-grok-test"')
    expect(grokConfig).toContain('api_backend = "responses"')

    const windowsTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows'
    )
    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('%userprofile%\\.grok/config.toml')

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )
    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const parsed = JSON.parse(wrapper.find('pre code').text())
    expect(parsed.provider.grok.npm).toBe('@ai-sdk/openai')
    expect(parsed.provider.grok.options).toEqual({
      baseURL: 'https://example.com/v1',
      apiKey: 'sk-grok-test'
    })
    expect(parsed.provider.grok.models['grok-4.5']).toBeDefined()
    expect(parsed.provider.grok.models['grok-build-0.1']).toBeDefined()
    expect(parsed.provider.grok.models['grok-composer-2.5-fast']).toBeDefined()
    expect(parsed.provider.grok.models['gpt-5.6']).toBeUndefined()
  })

  it('renders copyable Claude Code setup through the Grok Messages gateway', async () => {
    copyToClipboardMock.mockClear()
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-grok-claude-test',
        baseUrl: 'https://example.com/v1',
        platform: 'grok'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const claudeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.claudeCode')
    )
    expect(claudeTab).toBeDefined()
    await claudeTab!.trigger('click')
    await nextTick()

    let codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(codeBlocks.join('\n')).toContain('ANTHROPIC_BASE_URL="https://example.com"')
    expect(codeBlocks.join('\n')).toContain('ANTHROPIC_AUTH_TOKEN="sk-grok-claude-test"')
    const unixConfig = codeBlocks.find((content) => content.startsWith('export ANTHROPIC_BASE_URL'))
    expect(unixConfig).toBeDefined()
    for (const name of [
      'ANTHROPIC_MODEL',
      'ANTHROPIC_DEFAULT_OPUS_MODEL',
      'ANTHROPIC_DEFAULT_SONNET_MODEL',
      'ANTHROPIC_DEFAULT_HAIKU_MODEL',
      'ANTHROPIC_DEFAULT_FABLE_MODEL',
      'CLAUDE_CODE_SUBAGENT_MODEL'
    ]) {
      expect(unixConfig).toContain(`export ${name}="grok-4.5"`)
    }
    const settingsConfig = codeBlocks.find((content) => content.includes('"$schema"'))
    expect(settingsConfig).toBeDefined()
    const parsedSettings = JSON.parse(settingsConfig!)
    expect(parsedSettings.$schema).toBe('https://json.schemastore.org/claude-code-settings.json')
    expect(parsedSettings.env.ANTHROPIC_MODEL).toBe('grok-4.5')
    expect(wrapper.text()).toContain('keys.useKeyModal.claudeSettingsHint')
    expect(wrapper.text()).toContain('keys.useKeyModal.grok.claudeNote')
    expect(wrapper.find('nav[aria-label="Client"]').classes()).toContain('min-w-max')
    expect(wrapper.find('nav[aria-label="Client"]').element.parentElement?.classList.contains('overflow-x-auto')).toBe(true)

    const cmdTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows CMD'
    )
    expect(cmdTab).toBeDefined()
    await cmdTab!.trigger('click')
    await nextTick()

    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(codeBlocks.join('\n')).toContain('set ANTHROPIC_MODEL=grok-4.5')
    expect(codeBlocks.join('\n')).toContain('set ANTHROPIC_DEFAULT_FABLE_MODEL=grok-4.5')
    expect(codeBlocks.join('\n')).toContain('set CLAUDE_CODE_SUBAGENT_MODEL=grok-4.5')

    const powershellTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'PowerShell'
    )
    expect(powershellTab).toBeDefined()
    await powershellTab!.trigger('click')
    await nextTick()

    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(codeBlocks.join('\n')).toContain('$env:ANTHROPIC_BASE_URL="https://example.com"')
    expect(codeBlocks.join('\n')).toContain('$env:ANTHROPIC_MODEL="grok-4.5"')
    expect(codeBlocks.join('\n')).toContain('$env:ANTHROPIC_DEFAULT_FABLE_MODEL="grok-4.5"')
    expect(codeBlocks.join('\n')).toContain('$env:CLAUDE_CODE_SUBAGENT_MODEL="grok-4.5"')
    expect(wrapper.text()).toContain('%USERPROFILE%\\.claude\\settings.json')

    const copyButton = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.copy')
    )
    expect(copyButton).toBeDefined()
    await copyButton!.trigger('click')
    expect(copyToClipboardMock).toHaveBeenCalledWith(
      expect.stringContaining('ANTHROPIC_AUTH_TOKEN="sk-grok-claude-test"'),
      'keys.copied'
    )
  })

  it('renders Codex custom provider setup through the Grok Responses gateway', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-grok-codex-test',
        baseUrl: 'https://example.com/v1',
        platform: 'grok'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const codexTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCli')
    )
    expect(codexTab).toBeDefined()
    await codexTab!.trigger('click')
    await nextTick()

    let codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('[model_providers.sub2api_grok]'))
    expect(configToml).toBeDefined()
    expect(configToml).toContain('model_provider = "sub2api_grok"')
    expect(configToml).toContain('model = "grok-4.5"')
    expect(configToml).toContain('model_context_window = 1000000')
    expect(configToml).toContain('base_url = "https://example.com/v1"')
    expect(configToml).toContain('env_key = "SUB2API_API_KEY"')
    expect(configToml).toContain('wire_api = "responses"')
    expect(configToml).toContain('supports_websockets = true')
    expect(configToml).not.toContain('requires_openai_auth')
    expect(configToml).not.toContain('disable_response_storage')
    expect(configToml).not.toContain('network_access')
    expect(configToml).not.toContain('windows_wsl_setup_acknowledged')
    expect(configToml).toContain('[features]\nresponses_websockets_v2 = true')
    expect(configToml).not.toContain('goals = true')
    expect(codeBlocks).toContain('export SUB2API_API_KEY="sk-grok-codex-test"')
    expect(wrapper.text()).not.toContain('auth.json')

    const windowsTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows'
    )
    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()

    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(wrapper.text()).toContain('%USERPROFILE%\\.codex\\config.toml')
    expect(codeBlocks).toContain('$env:SUB2API_API_KEY="sk-grok-codex-test"')
  })

  it('keeps legacy OpenAI Codex config as the default', () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('model_provider = "OpenAI"'))

    expect(configToml).toBeDefined()
    expect(configToml).toContain('model = "gpt-5.5"')
    expect(configToml).toContain('review_model = "gpt-5.5"')
    expect(configToml).not.toContain('model = "gpt-5.4"')
    expect(configToml).not.toContain('model_context_window')
    expect(configToml).not.toContain('model_auto_compact_token_limit')
    expect(configToml).toContain('requires_openai_auth = true')
    expect(configToml).not.toContain('x-openai-actor-authorization')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(configToml).not.toContain('supports_websockets')
    expect(configToml).not.toContain('responses_websockets_v2')
    expect(configToml).toContain('[features]\ngoals = true')
    expect(codeBlocks).toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
    expect(wrapper.text()).toContain('auth.json')
    expect(wrapper.find('[data-testid="codex-api-key-restart-notice"]').exists()).toBe(false)
  })

  it('renders API Key Mode authorization in OpenAI Codex config', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const apiKeyMode = wrapper.get('[data-testid="codex-auth-mode-api-key"]')
    await apiKeyMode.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('model_provider = "OpenAI"'))

    expect(apiKeyMode.attributes('aria-checked')).toBe('true')
    expect(configToml).toBeDefined()
    expect(configToml).toContain('requires_openai_auth = false')
    expect(configToml).toContain('http_headers = { "x-openai-actor-authorization" = "local-image-extension" }')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(codeBlocks).toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
    expect(wrapper.text()).toContain('auth.json')

    const restartNotice = wrapper.get('[data-testid="codex-api-key-restart-notice"]')
    expect(restartNotice.text()).toContain(
      'keys.useKeyModal.openai.authModeApiKeyRestartNotice'
    )

    await wrapper.get('[data-testid="codex-auth-mode-legacy"]').trigger('click')
    await nextTick()

    expect(wrapper.find('[data-testid="codex-api-key-restart-notice"]').exists()).toBe(false)
    expect(wrapper.findAll('pre code').map((code) => code.text()).join('\n')).not.toContain(
      'x-openai-actor-authorization'
    )
  })

  it('keeps legacy OpenAI Codex WebSocket config as the default', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const wsTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCliWs')
    )

    expect(wsTab).toBeDefined()
    await wsTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('supports_websockets = true'))

    expect(configToml).toBeDefined()
    expect(configToml).toContain('model = "gpt-5.5"')
    expect(configToml).toContain('review_model = "gpt-5.5"')
    expect(configToml).not.toContain('model = "gpt-5.4"')
    expect(configToml).not.toContain('model_context_window')
    expect(configToml).not.toContain('model_auto_compact_token_limit')
    expect(configToml).toContain('requires_openai_auth = true')
    expect(configToml).not.toContain('x-openai-actor-authorization')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(configToml).toContain('supports_websockets = true')
    expect(configToml).toContain('[features]\nresponses_websockets_v2 = true\ngoals = true')
    expect(codeBlocks).toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
    expect(wrapper.text()).toContain('auth.json')
  })

  it('preserves API Key Mode when switching to OpenAI Codex WebSocket config', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const apiKeyMode = wrapper.get('[data-testid="codex-auth-mode-api-key"]')
    await apiKeyMode.trigger('click')

    const wsTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCliWs')
    )
    expect(wsTab).toBeDefined()
    await wsTab!.trigger('click')
    await nextTick()

    const codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) => content.includes('supports_websockets = true'))

    expect(wrapper.get('[data-testid="codex-auth-mode-api-key"]').attributes('aria-checked')).toBe('true')
    expect(configToml).toBeDefined()
    expect(configToml).toContain('requires_openai_auth = false')
    expect(configToml).toContain('http_headers = { "x-openai-actor-authorization" = "local-image-extension" }')
    expect(configToml).not.toContain('env_key')
    expect(configToml).not.toContain('image_generation')
    expect(configToml).toContain('supports_websockets = true')
    expect(configToml).toContain('[features]\nresponses_websockets_v2 = true\ngoals = true')
    expect(codeBlocks).toContain('{\n  "OPENAI_API_KEY": "sk-test"\n}')
  })

  it('resets Codex authentication mode when the modal reopens or platform changes', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    await wrapper.get('[data-testid="codex-auth-mode-api-key"]').trigger('click')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await nextTick()

    expect(wrapper.get('[data-testid="codex-auth-mode-legacy"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.findAll('pre code').map((code) => code.text()).join('\n')).toContain('requires_openai_auth = true')

    await wrapper.get('[data-testid="codex-auth-mode-api-key"]').trigger('click')
    await wrapper.setProps({ platform: 'gemini' })
    await wrapper.setProps({ platform: 'openai' })
    await nextTick()

    expect(wrapper.get('[data-testid="codex-auth-mode-legacy"]').attributes('aria-checked')).toBe('true')
    expect(wrapper.findAll('pre code').map((code) => code.text()).join('\n')).not.toContain('x-openai-actor-authorization')
  })

  it('renders GPT-5.4 mini entry in OpenCode config', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )

    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const codeBlock = wrapper.find('pre code')
    expect(codeBlock.exists()).toBe(true)
    expect(codeBlock.text()).toContain('"name": "GPT-5.4 Mini"')
    expect(codeBlock.text()).not.toContain('"name": "GPT-5.4 Nano"')
  })

  it('renders GPT-5.6 alias and max variants in OpenCode config', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'openai'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )
    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const parsed = JSON.parse(wrapper.find('pre code').text())
    const models = parsed.provider.openai.models
    for (const model of ['gpt-5.6', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna']) {
      expect(models[model]).toBeDefined()
      expect(models[model].variants).toHaveProperty('max')
      expect(models[model].variants).toHaveProperty('xhigh')
    }
    expect(models['gpt-5.6'].name).toBe('GPT-5.6 (Sol)')
  })

  it('renders Claude Fable 5 OpenCode config with adaptive thinking', async () => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-test',
        baseUrl: 'https://example.com/v1',
        platform: 'antigravity'
      },
      global: {
        stubs: {
          BaseDialog: {
            template: '<div><slot /><slot name="footer" /></div>'
          },
          Icon: {
            template: '<span />'
          }
        }
      }
    })

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )

    expect(opencodeTab).toBeDefined()
    await opencodeTab!.trigger('click')
    await nextTick()

    const claudeConfig = wrapper.findAll('pre code')
      .map((code) => code.text())
      .find((content) => content.includes('"antigravity-claude"'))

    expect(claudeConfig).toBeDefined()
    const parsed = JSON.parse(claudeConfig!)
    const fable = parsed.provider['antigravity-claude'].models['claude-fable-5']

    expect(fable.name).toBe('Claude Fable 5')
    expect(fable.limit).toEqual({ context: 1048576, output: 128000 })
    expect(fable.options.thinking).toEqual({ type: 'adaptive' })
    expect(fable.options.thinking).not.toHaveProperty('budgetTokens')
  })

  it('renders DeepSeek OpenCode and Continue configs for Unix and Windows', async () => {
    const wrapper = mountDeepSeekModal()

    let parsed = JSON.parse(wrapper.find('pre code').text())
    expect(parsed.provider.deepseek.npm).toBe('@ai-sdk/openai-compatible')
    expect(parsed.provider.deepseek.options).toEqual({
      baseURL: 'https://modelport.example/v1',
      apiKey: 'sk-modelport-deepseek-test'
    })
    expect(parsed.provider.deepseek.models['deepseek-chat']).toBeDefined()
    expect(parsed.provider.deepseek.models['deepseek-reasoner']).toBeDefined()
    expect(wrapper.text()).toContain('~/.config/opencode/opencode.json')

    const windowsTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows'
    )
    expect(windowsTab).toBeDefined()
    await windowsTab!.trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('%USERPROFILE%\\.config\\opencode\\opencode.json')

    const continueTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.continue')
    )
    expect(continueTab).toBeDefined()
    await continueTab!.trigger('click')
    await nextTick()

    const config = wrapper.find('pre code').text()
    expect(wrapper.text()).toContain('~/.continue/config.yaml')
    expect(config).toContain('provider: openai')
    expect(config).toContain('model: deepseek-chat')
    expect(config).toContain('model: deepseek-reasoner')
    expect(config).toContain('apiBase: "https://modelport.example/v1"')
    expect(config).toContain('apiKey: "sk-modelport-deepseek-test"')

    const continueWindowsTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows'
    )
    await continueWindowsTab!.trigger('click')
    await nextTick()
    expect(wrapper.text()).toContain('%USERPROFILE%\\.continue\\config.yaml')

    const opencodeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.opencode')
    )
    await opencodeTab!.trigger('click')
    await nextTick()
    parsed = JSON.parse(wrapper.find('pre code').text())
    expect(parsed.provider.deepseek.models['deepseek-chat']).toEqual({ name: 'deepseek-chat' })
  })

  it('renders DeepSeek Claude Code and Codex CLI configs across systems', async () => {
    const wrapper = mountDeepSeekModal()
    const claudeTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.claudeCode')
    )
    await claudeTab!.trigger('click')
    await nextTick()

    let codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(codeBlocks.join('\n')).toContain('export ANTHROPIC_BASE_URL="https://modelport.example"')
    expect(codeBlocks.join('\n')).toContain('export ANTHROPIC_AUTH_TOKEN="sk-modelport-deepseek-test"')
    expect(codeBlocks.join('\n')).toContain('export ANTHROPIC_MODEL="deepseek-chat"')
    expect(codeBlocks.join('\n')).toContain('export CLAUDE_CODE_SUBAGENT_MODEL="deepseek-chat"')

    const cmdTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows CMD'
    )
    await cmdTab!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('pre code').map((code) => code.text()).join('\n'))
      .toContain('set ANTHROPIC_MODEL=deepseek-chat')

    const codexTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCli')
    )
    await codexTab!.trigger('click')
    await nextTick()

    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    const configToml = codeBlocks.find((content) =>
      content.includes('[model_providers.modelport_deepseek]')
    )
    expect(configToml).toContain('model_provider = "modelport_deepseek"')
    expect(configToml).toContain('model = "deepseek-chat"')
    expect(configToml).toContain('base_url = "https://modelport.example/v1"')
    expect(configToml).toContain('env_key = "MODELPORT_API_KEY"')
    expect(configToml).toContain('wire_api = "responses"')
    expect(configToml).not.toContain('supports_websockets')
    expect(codeBlocks).toContain('export MODELPORT_API_KEY="sk-modelport-deepseek-test"')

    const codexWindowsTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Windows'
    )
    await codexWindowsTab!.trigger('click')
    await nextTick()
    codeBlocks = wrapper.findAll('pre code').map((code) => code.text())
    expect(wrapper.text()).toContain('%USERPROFILE%\\.codex\\config.toml')
    expect(codeBlocks).toContain('$env:MODELPORT_API_KEY="sk-modelport-deepseek-test"')
  })

  it('renders DeepSeek SDK and HTTP examples for each supported shell', async () => {
    const wrapper = mountDeepSeekModal()
    const sdkTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.openaiSdk')
    )
    await sdkTab!.trigger('click')
    await nextTick()

    let content = wrapper.find('pre code').text()
    expect(wrapper.text()).toContain('example.py')
    expect(content).toContain('from openai import OpenAI')
    expect(content).toContain('base_url="https://modelport.example/v1"')
    expect(content).toContain('model="deepseek-chat"')
    expect(content).toContain('stream=True')

    const nodeTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'Node.js'
    )
    await nodeTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(wrapper.text()).toContain('example.mjs')
    expect(content).toContain('import OpenAI from "openai"')
    expect(content).toContain('baseURL: "https://modelport.example/v1"')
    expect(content).toContain('stream: true')

    const httpTab = wrapper.findAll('button').find((button) =>
      button.text().includes('keys.useKeyModal.cliTabs.httpApi')
    )
    await httpTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(content).toContain('curl -N --request POST "https://modelport.example/v1/chat/completions"')
    expect(content).toContain('Authorization: Bearer sk-modelport-deepseek-test')
    expect(content).toContain('"stream":true')

    const powershellTab = wrapper.findAll('button').find(
      (button) => button.text().trim() === 'PowerShell'
    )
    await powershellTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(content).toContain('curl.exe -N --request POST "https://modelport.example/v1/chat/completions"')
    expect(content).toContain('"stream":true')
  })

  it.each([
    ['qwen', 'qwen3.7-plus', 'qwen3.8-max-preview'],
    ['glm', 'glm-5.2', 'glm-5.2'],
    ['kimi', 'kimi-k3', 'kimi-k3'],
    ['doubao', 'YOUR_ARK_ENDPOINT_ID', 'doubao-seed-1.8'],
    ['siliconflow', 'deepseek-ai/DeepSeek-V3.2', 'deepseek-ai/DeepSeek-V3.2'],
    ['openrouter', 'openai/gpt-4o-mini', 'openai/gpt-4o-mini'],
    ['minimax', 'MiniMax-M3', 'MiniMax-M3'],
    ['mimo', 'mimo-v2.5', 'mimo-v2.5-pro']
  ])('renders the full compatible client matrix for %s', async (platform, defaultModel, suggestion) => {
    const wrapper = mount(UseKeyModal, {
      props: {
        show: true,
        apiKey: 'sk-compatible-test',
        baseUrl: 'https://modelport.example/v1',
        platform: platform as any
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: { template: '<span />' }
        }
      }
    })

    const openCodeConfig = JSON.parse(wrapper.find('pre code').text())
    expect(openCodeConfig.provider[platform].models[suggestion]).toEqual({ name: suggestion })
    expect(openCodeConfig.provider[platform].models[suggestion]).not.toHaveProperty('limit')

    const continueTab = wrapper.findAll('button').find(button =>
      button.text().includes('keys.useKeyModal.cliTabs.continue')
    )
    expect(continueTab).toBeDefined()
    await continueTab!.trigger('click')
    await nextTick()
    let content = wrapper.find('pre code').text()
    expect(content).toContain(`model: ${suggestion}`)
    expect(content).toContain('apiBase: "https://modelport.example/v1"')

    const sdkTab = wrapper.findAll('button').find(button =>
      button.text().includes('keys.useKeyModal.cliTabs.openaiSdk')
    )
    expect(sdkTab).toBeDefined()
    await sdkTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(content).toContain('from openai import OpenAI')
    expect(content).toContain(`model="${defaultModel}"`)
    expect(content).toContain('stream=True')
    expect(content).toContain('flush=True')

    const nodeTab = wrapper.findAll('button').find(button => button.text().trim() === 'Node.js')
    expect(nodeTab).toBeDefined()
    await nodeTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(content).toContain('import OpenAI from "openai"')
    expect(content).toContain(`model: "${defaultModel}"`)
    expect(content).toContain('stream: true')
    expect(content).toContain('for await (const chunk of stream)')

    const codexTab = wrapper.findAll('button').find(button =>
      button.text().includes('keys.useKeyModal.cliTabs.codexCli')
    )
    expect(codexTab).toBeDefined()
    await codexTab!.trigger('click')
    await nextTick()
    expect(wrapper.findAll('pre code').map(code => code.text()).join('\n')).toContain(`model = "${defaultModel}"`)

    const httpTab = wrapper.findAll('button').find(button =>
      button.text().includes('keys.useKeyModal.cliTabs.httpApi')
    )
    await httpTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(content).toContain('curl -N --request POST')
    expect(content).toContain(`"model":"${defaultModel}"`)
    expect(content).toContain('"stream":true')

    const cmdTab = wrapper.findAll('button').find(button => button.text().trim() === 'Windows CMD')
    expect(cmdTab).toBeDefined()
    await cmdTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(content).toContain('curl -N --request POST')
    expect(content).toContain('\\"stream\\":true')

    const powershellTab = wrapper.findAll('button').find(button => button.text().trim() === 'PowerShell')
    expect(powershellTab).toBeDefined()
    await powershellTab!.trigger('click')
    await nextTick()
    content = wrapper.find('pre code').text()
    expect(content).toContain('curl.exe -N --request POST')
    expect(content).toContain('"stream":true')

    wrapper.unmount()
  })
})
