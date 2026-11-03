export interface AgentModel {
  id: string
  name: string
  provider: string
  enabled: boolean
  has_api_key?: boolean
  base_url?: string
  model_name?: string
  // What it calls unless given a name of its own.
  default_model?: string
  // On a supported provider: what it uses unless told otherwise.
  default_base_url?: string
}

export interface UpdateModelConfigRequest {
  models: {
    [key: string]: {
      enabled: boolean
      api_key: string
      base_url?: string
      model_name?: string
    }
  }
}

export interface TestModelConfigRequest {
  agent_model_id: string
  api_key: string
  base_url: string
  model_name: string
}

export interface ModelTest {
  ok: boolean
  model: string
  latency_ms?: number
  error?: string
}
