/** Edge of the square the cropped avatar is uploaded at; the server keeps it. */
const AVATAR_SIZE = 256

/** Longest edge a picked image is worked on at; plenty for a 256px avatar. */
const WORKING_SIZE = 1024

/** The original never leaves the browser, so this only bounds memory use. */
const MAX_AVATAR_FILE = 10 << 20

export const AVATAR_TYPES = ['image/png', 'image/jpeg', 'image/webp']

/** A region of an image, in its natural pixels. */
export interface CropArea {
  x: number
  y: number
  width: number
  height: number
}

/**
 * The translation key of the reason a picked file is refused, or null when it
 * can go on to cropping. The server checks the actual bytes again.
 */
export function checkAvatarFile(
  file: File
): 'avatarWrongType' | 'avatarTooLarge' | null {
  if (!AVATAR_TYPES.includes(file.type)) return 'avatarWrongType'
  if (file.size > MAX_AVATAR_FILE) return 'avatarTooLarge'
  return null
}

/** Loads an image URL, rejecting when the browser cannot decode it. */
async function loadImage(src: string): Promise<HTMLImageElement> {
  const image = new Image()
  image.src = src
  await image.decode()
  return image
}

function toBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) =>
    canvas.toBlob(
      (blob) =>
        blob ? resolve(blob) : reject(new Error('Could not encode the image')),
      'image/png'
    )
  )
}

function newCanvas(width: number, height: number) {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const context = canvas.getContext('2d')!
  context.imageSmoothingQuality = 'high'
  return { canvas, context }
}

/**
 * Decodes a picked file and redraws it at most 1024px on its longest edge, so
 * cropping and rotating stay quick. Returns an object URL the caller revokes;
 * rejects when the browser cannot decode the file.
 */
export async function prepareAvatarSource(file: File): Promise<string> {
  const original = URL.createObjectURL(file)
  try {
    const image = await loadImage(original)
    const scale = Math.min(
      1,
      WORKING_SIZE / Math.max(image.naturalWidth, image.naturalHeight)
    )
    const { canvas, context } = newCanvas(
      Math.round(image.naturalWidth * scale),
      Math.round(image.naturalHeight * scale)
    )
    context.drawImage(image, 0, 0, canvas.width, canvas.height)
    return URL.createObjectURL(await toBlob(canvas))
  } finally {
    URL.revokeObjectURL(original)
  }
}

/** Turns an image a quarter turn either way; returns a new object URL. */
export async function rotateQuarter(
  src: string,
  direction: 'left' | 'right'
): Promise<string> {
  const image = await loadImage(src)
  const { canvas, context } = newCanvas(image.naturalHeight, image.naturalWidth)
  if (direction === 'left') {
    context.translate(0, canvas.height)
    context.rotate(-Math.PI / 2)
  } else {
    context.translate(canvas.width, 0)
    context.rotate(Math.PI / 2)
  }
  context.drawImage(image, 0, 0)
  return URL.createObjectURL(await toBlob(canvas))
}

/** Draws the cropped area onto a 256px square and returns it as a PNG. */
export async function renderAvatar(src: string, area: CropArea): Promise<Blob> {
  const image = await loadImage(src)
  const { canvas, context } = newCanvas(AVATAR_SIZE, AVATAR_SIZE)
  context.drawImage(
    image,
    area.x,
    area.y,
    area.width,
    area.height,
    0,
    0,
    AVATAR_SIZE,
    AVATAR_SIZE
  )
  return toBlob(canvas)
}
