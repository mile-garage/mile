import { useRef, useState } from 'react'
import { api, ApiError, fileUrl, type Attachment } from '../api'
import { useI18n } from '../i18n'
import { bytes } from '../format'
import { Icon } from './Icon'
import { Spinner, useUI } from './ui'

const MAX_SIDE = { photo: 2000, document: 2600 }

/**
 * Photos are resized and re-encoded as JPEG in the browser: smaller uploads,
 * no EXIF location data, and HEIC from iPhones (which Safari decodes) becomes
 * a format every browser can show. PDFs are sent as they are.
 */
async function prepare(file: File, kind: 'photo' | 'document'): Promise<Blob> {
  const isImage = file.type.startsWith('image/') || /\.(heic|heif)$/i.test(file.name)
  if (!isImage) return file
  let bmp: ImageBitmap
  try {
    bmp = await createImageBitmap(file, { imageOrientation: 'from-image' })
  } catch {
    throw new ApiError(400, 'heic', 'heic')
  }
  const scale = Math.min(1, MAX_SIDE[kind] / Math.max(bmp.width, bmp.height))
  const canvas = document.createElement('canvas')
  canvas.width = Math.round(bmp.width * scale)
  canvas.height = Math.round(bmp.height * scale)
  canvas.getContext('2d')!.drawImage(bmp, 0, 0, canvas.width, canvas.height)
  bmp.close()
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('encode'))), 'image/jpeg', kind === 'photo' ? 0.85 : 0.9),
  )
}

export interface UploadTarget {
  vehicleId: number
  kind: 'photo' | 'document'
  expense_id?: number
  refuel_id?: number
  policy_id?: number
}

export async function upload(target: UploadTarget, file: File): Promise<Attachment> {
  const blob = await prepare(file, target.kind)
  const fd = new FormData()
  fd.append('kind', target.kind)
  if (target.expense_id) fd.append('expense_id', String(target.expense_id))
  if (target.refuel_id) fd.append('refuel_id', String(target.refuel_id))
  if (target.policy_id) fd.append('policy_id', String(target.policy_id))
  const name = blob === file ? file.name : file.name.replace(/\.[^.]+$/, '') + '.jpg'
  fd.append('file', blob, name)
  return api.post<Attachment>(`/api/vehicles/${target.vehicleId}/attachments`, fd)
}

/** Button that uploads one or more files; the camera is offered on phones. */
export function UploadButton({
  target,
  onUploaded,
  label,
  multiple,
  className = 'btn btn-sm',
}: {
  target: UploadTarget
  onUploaded: (a: Attachment) => void
  label?: string
  multiple?: boolean
  className?: string
}) {
  const { t } = useI18n()
  const { toast, fail } = useUI()
  const input = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)

  const onFiles = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    for (const f of Array.from(files)) {
      try {
        onUploaded(await upload(target, f))
      } catch (e) {
        if (e instanceof ApiError && e.code === 'heic') toast(t('files.heic'), 'error')
        else fail(e)
      }
    }
    setBusy(false)
    if (input.current) input.current.value = ''
  }

  return (
    <>
      <button type="button" className={className} disabled={busy} onClick={() => input.current?.click()}>
        {busy ? <Spinner small /> : <Icon name={target.kind === 'photo' ? 'camera' : 'paperclip'} size={16} />}
        {busy ? t('files.uploading') : (label ?? t('files.attach'))}
      </button>
      <input
        ref={input}
        type="file"
        hidden
        multiple={multiple}
        accept={target.kind === 'photo' ? 'image/*' : 'application/pdf,image/*'}
        onChange={(e) => onFiles(e.target.files)}
      />
    </>
  )
}

/** Links to the attached documents, with delete. */
export function FileList({
  files,
  canEdit,
  onDeleted,
}: {
  files: Attachment[]
  canEdit: boolean
  onDeleted: (id: number) => void
}) {
  const { t } = useI18n()
  const { confirm, fail } = useUI()
  if (files.length === 0) return null
  const del = async (a: Attachment) => {
    if (!(await confirm({ title: a.file_name, message: t('files.deleteText'), confirmLabel: t('common.delete'), danger: true }))) return
    try {
      await api.del(`/api/attachments/${a.id}`)
      onDeleted(a.id)
    } catch (e) {
      fail(e)
    }
  }
  return (
    <ul className="files">
      {files.map((a) => (
        <li key={a.id}>
          <a href={fileUrl(a.id)} target="_blank" rel="noopener">
            <Icon name="file" size={16} />
            <span className="file-name">{a.file_name}</span>
            <span className="muted">{bytes(a.size)}</span>
          </a>
          {canEdit && (
            <button type="button" className="btn-icon" onClick={() => del(a)} aria-label={t('common.delete')}>
              <Icon name="trash" size={16} />
            </button>
          )}
        </li>
      ))}
    </ul>
  )
}
