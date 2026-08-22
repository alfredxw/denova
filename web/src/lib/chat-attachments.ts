export const MAX_CHAT_ATTACHMENT_FILES = 10
export const MAX_CHAT_ATTACHMENT_BYTES = 20 * 1024 * 1024
export const MAX_CHAT_ATTACHMENTS_BYTES = 50 * 1024 * 1024

export interface ChatAttachmentUpload {
  name: string
  media_type?: string
  data_url: string
}

export interface ChatAttachmentDescriptor {
  id?: string
  name: string
  media_type?: string
  size: number
}

export async function filesToAttachmentUploads(files: File[]): Promise<ChatAttachmentUpload[]> {
  return Promise.all(files.map(async (file) => ({
    name: file.name,
    media_type: file.type || undefined,
    data_url: await readFileDataURL(file),
  })))
}

export function attachmentDescriptors(files: File[]): ChatAttachmentDescriptor[] {
  return files.map((file) => ({
    name: file.name,
    media_type: file.type || undefined,
    size: file.size,
  }))
}

// Keep base64 payloads out of command retry-map keys. The server's
// content-derived attachment IDs remain the authoritative conflict check.
export function attachmentUploadsRetryIdentity(uploads: ChatAttachmentUpload[]) {
  return uploads.map((upload) => ({
    name: upload.name,
    media_type: upload.media_type,
    encoded_length: upload.data_url.length,
  }))
}

function readFileDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error || new Error(`Failed to read attachment: ${file.name}`))
    reader.onload = () => typeof reader.result === 'string'
      ? resolve(reader.result)
      : reject(new Error(`Failed to encode attachment: ${file.name}`))
    reader.readAsDataURL(file)
  })
}
