export type SemanticAnchor = {
  name: string
  route_group: string
  preferred_models?: string[]
  modality?: string
  vector_dims?: number
  anchor_text?: string | null
}

export type AdminModelOption = {
  id: string
  provider: string
  route_groups?: string[]
}

export type SemanticAnchorListResponse = {
  data: SemanticAnchor[]
  tenant_id?: string
  pagination?: {
    limit?: number
    offset?: number
    total?: number
  }
}

export type SemanticTestResponse = {
  input: string
  threshold: number
  top_match: {
    anchor: string
    route_group: string
    similarity: number
    passed: boolean
  } | null
  decision: {
    type: 'semantic_anchor' | 'none'
    result: string | null
  }
}

export type SemanticSuggestion = {
  anchor: string
  examples: number
}

export type SemanticSuggestionResponse = {
  anchors: SemanticSuggestion[]
}

export type SemanticCalibrationResponse = {
  recommended_threshold: number
  precision: number
  recall: number
  f1: number
}

export type SemanticRoute = {
  name: string
  description?: string
  action: string
  utterances?: string[] | null
  threshold?: number
}

export type SemanticRouteListResponse = {
  routes: SemanticRoute[]
}
