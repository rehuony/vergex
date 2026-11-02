export interface AgentModel {
  id: string
  name: string
  provider: string
  enabled: boolean
  has_api_key?: boolean
  apiKey?: string
  customApiUrl?: string
  customModelName?: string
  // On a supported provider: what it uses unless told otherwise.
  defaultBaseUrl?: string
}

export interface UpdateModelConfigRequest {
  models: {
    [key: string]: {
      enabled: boolean
      api_key: string
      custom_api_url?: string
      custom_model_name?: string
    }
  }
}

export interface TestModelConfigRequest {
  agent_model_id: string
  api_key: string
  custom_api_url: string
  custom_model_name: string
}

export interface ModelTest {
  ok: boolean
  model: string
  latency_ms?: number
  error?: string
}
