export const WHITE_LABEL = process.env.NEXT_PUBLIC_WHITE_LABEL?.trim() === 'true'

// Only meaningful when WHITE_LABEL=true — shows custom text instead of logos
export const BRAND_NAME = process.env.NEXT_PUBLIC_BRAND_NAME ?? ''
export const BRAND_SUBTITLE = process.env.NEXT_PUBLIC_BRAND_SUBTITLE ?? ''
