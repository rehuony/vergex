import { MonoIcon } from './mono-icon'

interface IconProps {
  width?: number
  height?: number
  className?: string
}

const MODEL_ICONS = new Set([
  'deepseek',
  'qwen',
  'claude',
  'kimi',
  'gemini',
  'grok',
  'openai',
  'minimax',
])

// Returns the icon for a model
export const getModelIcon = (modelType: string, props: IconProps = {}) => {
  // Supports full ID or type name
  const type = modelType.includes('_') ? modelType.split('_').pop() : modelType
  if (!type || !MODEL_ICONS.has(type)) return null

  return (
    <MonoIcon
      src={`/icons/${type}.svg`}
      label={`${type} icon`}
      width={props.width}
      height={props.height}
      className={props.className}
    />
  )
}
