import { useEffect, useRef, useState } from 'react'
import { ImagePlus, Loader2, X } from 'lucide-react'
import { useNavigate, useParams } from 'react-router'
import TagInput from '../components/TagInput'
import {
  useCategories,
  useCreatePost,
  usePost,
  usePublishPost,
  useUpdatePost,
  useUploadPostImage,
} from '../lib/queries'

export default function PostEditor() {
  const { id } = useParams()
  const postId = id ? Number(id) : undefined
  const isEdit = postId !== undefined
  const navigate = useNavigate()

  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [selectedCategories, setSelectedCategories] = useState<number[]>([])
  const [tags, setTags] = useState<string[]>([])
  const [imageUrl, setImageUrl] = useState<string | null>(null)
  const [validationError, setValidationError] = useState<string | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  const { data: categories } = useCategories()
  const existing = usePost(postId ?? 0)
  const create = useCreatePost()
  const update = useUpdatePost(postId ?? 0)
  const publish = usePublishPost()
  const uploadImage = useUploadPostImage()
  const isDraft = isEdit ? (existing.data?.post.draft ?? false) : true

  // Prefill the form when editing.
  useEffect(() => {
    if (isEdit && existing.data) {
      setTitle(existing.data.post.title)
      setContent(existing.data.post.content)
      setSelectedCategories(existing.data.post.categories?.map((c) => c.id) ?? [])
    }
  }, [isEdit, existing.data])

  if (isEdit && existing.isLoading) {
    return (
      <div className="flex justify-center py-16">
        <Loader2 className="animate-spin text-zinc-400" />
      </div>
    )
  }

  const toggleCategory = (categoryId: number) => {
    setSelectedCategories((prev) =>
      prev.includes(categoryId) ? prev.filter((c) => c !== categoryId) : [...prev, categoryId],
    )
  }

  const mutation = isEdit ? update : create

  const submit = (asDraft: boolean) => {
    const trimmedTitle = title.trim()
    const trimmedContent = content.trim()
    if (!trimmedTitle || !trimmedContent) {
      setValidationError('Title and content are required.')
      return
    }
    if (!isEdit && selectedCategories.length === 0) {
      setValidationError('Pick at least one category.')
      return
    }
    setValidationError(null)

    if (isEdit) {
      update.mutate(
        { title: trimmedTitle, content: trimmedContent },
        {
          onSuccess: () => {
            if (isDraft && !asDraft) {
              publish.mutate(postId!, { onSuccess: () => navigate(`/post/${postId}`) })
            } else {
              navigate(`/post/${postId}`)
            }
          },
        },
      )
    } else {
      create.mutate(
        {
          title: trimmedTitle,
          content: trimmedContent,
          category_ids: selectedCategories,
          image_url: imageUrl ?? undefined,
          tags: tags.length > 0 ? tags : undefined,
          draft: asDraft,
        },
        {
          onSuccess: ({ id: newId }) =>
            navigate(asDraft ? `/post/${newId}/edit` : `/post/${newId}`),
        },
      )
    }
  }

  const error = validationError ?? (mutation.isError ? mutation.error.message : publish.isError ? publish.error.message : null)

  return (
    <div className="mx-auto max-w-2xl">
      <h1 className="text-2xl font-bold">{isEdit ? 'Edit post' : 'New post'}</h1>
      <form
        className="mt-6 space-y-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit(false)
        }}
      >
        {error && (
          <p className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/50 dark:text-red-300">
            {error}
          </p>
        )}
        <label className="block">
          <span className="mb-1 block text-sm font-medium">Title</span>
          <input
            type="text"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            maxLength={100}
            required
            autoFocus={!isEdit}
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-sm font-medium">
            Content <span className="font-normal text-zinc-400">(markdown supported)</span>
          </span>
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            rows={10}
            required
            className="w-full rounded-lg border border-zinc-300 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-zinc-700 dark:bg-zinc-900"
          />
        </label>
        {!isEdit && (
          <>
            <fieldset>
              <legend className="mb-1 text-sm font-medium">Categories</legend>
              <div className="flex flex-wrap gap-2">
                {(categories ?? []).map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => toggleCategory(c.id)}
                    className={`rounded-full px-3 py-1 text-sm font-medium transition ${
                      selectedCategories.includes(c.id)
                        ? 'bg-indigo-600 text-white'
                        : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-300 dark:hover:bg-zinc-700'
                    }`}
                  >
                    {c.name}
                  </button>
                ))}
              </div>
            </fieldset>

            <div>
              <span className="mb-1 block text-sm font-medium">
                Tags <span className="font-normal text-zinc-400">(up to 5, optional)</span>
              </span>
              <TagInput tags={tags} onChange={setTags} />
            </div>

            <div>
              <span className="mb-1 block text-sm font-medium">
                Image <span className="font-normal text-zinc-400">(optional)</span>
              </span>
              {imageUrl ? (
                <div className="relative inline-block">
                  <img src={imageUrl} alt="" className="max-h-48 rounded-lg border border-zinc-200 dark:border-zinc-800" />
                  <button
                    type="button"
                    onClick={() => setImageUrl(null)}
                    aria-label="Remove image"
                    className="absolute -top-2 -right-2 rounded-full bg-zinc-900 p-1 text-white shadow hover:bg-zinc-700"
                  >
                    <X size={14} />
                  </button>
                </div>
              ) : (
                <div>
                  <input
                    ref={fileInput}
                    type="file"
                    accept="image/png,image/jpeg,image/gif,image/webp"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0]
                      if (file) uploadImage.mutate(file, { onSuccess: (data) => setImageUrl(data.url) })
                    }}
                  />
                  <button
                    type="button"
                    onClick={() => fileInput.current?.click()}
                    disabled={uploadImage.isPending}
                    className="flex items-center gap-1.5 rounded-lg border border-dashed border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                  >
                    {uploadImage.isPending ? (
                      <Loader2 size={15} className="animate-spin" />
                    ) : (
                      <ImagePlus size={15} />
                    )}
                    {uploadImage.isPending ? 'Uploading…' : 'Add image'}
                  </button>
                  {uploadImage.isError && (
                    <p className="mt-1 text-xs text-red-600 dark:text-red-400">{uploadImage.error.message}</p>
                  )}
                </div>
              )}
            </div>
          </>
        )}
        <div className="flex gap-2">
          {(!isEdit || isDraft) && (
            <button
              type="button"
              onClick={() => submit(true)}
              disabled={mutation.isPending || publish.isPending}
              className="rounded-lg border border-zinc-300 px-4 py-2 font-medium text-zinc-600 transition hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
            >
              {isEdit ? 'Save draft' : 'Save as draft'}
            </button>
          )}
          <button
            type="submit"
            disabled={mutation.isPending || publish.isPending}
            className="rounded-lg bg-indigo-600 px-4 py-2 font-medium text-white transition hover:bg-indigo-500 disabled:opacity-50"
          >
            {mutation.isPending || publish.isPending
              ? 'Saving…'
              : isEdit
                ? isDraft
                  ? 'Publish'
                  : 'Save changes'
                : 'Publish'}
          </button>
          <button
            type="button"
            onClick={() => navigate(-1)}
            className="rounded-lg px-4 py-2 font-medium text-zinc-600 transition hover:bg-zinc-100 dark:text-zinc-300 dark:hover:bg-zinc-800"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  )
}
