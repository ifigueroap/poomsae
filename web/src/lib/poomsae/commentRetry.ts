import type { PoomsaeCommentReplyIntent } from './types'

export function commentRetryIdentity(
  issueUID: string,
  body: string,
  reply?: PoomsaeCommentReplyIntent,
): string {
  return JSON.stringify([
    issueUID,
    body,
    reply?.replyTo ?? null,
    reply?.kind ?? null,
    reply?.force ?? false,
  ])
}
