package agentguide

// MCPCommentRules is CommentRules in tool terms.
const MCPCommentRules = `Comment with poomsae.comment and a stable idempotency_key only for findings a
later reader needs; keep live status in work.attention_msg. Correct your own
comment with poomsae.edit_comment. Record an open PR's URL in work.pr_url with
poomsae.set_metadata and attach it as pr evidence when closing. A PR that is open
and not merged is unfinished work. Leave the issue open,
update work.attention and work.attention_msg, and set the needs-review label.
`

// MCPClaimRules is ClaimRules in tool terms.
const MCPClaimRules = `Claim with poomsae.claim and ttl_seconds; renew before expiry using ttl_seconds,
without if_unowned.
`

// MCPContract is the compact bootstrap for clients that have tools and a file
// editor, but no shell. Richer guidance is fetched from poomsae.skills on demand.
const MCPContract = `Track real changes through Poomsae MCP, not questions, explanations or no-change
reviews.

Start with poomsae.skills {"name":"workflow"}; use
poomsae.skills {"name":"quickstart"} for examples and topic guides.
The connection fixes actor and project scope; read its server instructions.
Use project#ref for existing issues in multi-project scope. Supply project only
where the schema allows it; never guess another connection's scope.

Call poomsae.load_* and refresh the tool list before using its tools.
Search with poomsae.search before creating; reuse matching open work, or call
poomsae.create with a stable idempotency_key. Link related work or a parent.
Inspect/reuse look-alikes; force_new is for different work.
Create only requested work.
Never add p0-p4 or priority: labels; set priority.

` + MCPClaimRules + `Keep your own work.attention and work.attention_msg current through
poomsae.set_metadata. Stamp a dedicated work.branch once. Only write your own
work.*; ignore it on closed issues except work.pr_url. Do not take over a
sibling's issue or metadata. If blocked, explain what is needed.

` + MCPCommentRules + `

Close only verified work with poomsae.close: reason "done", a substantive message
of at least 40 characters, and typed evidence. An open child prevents parent
closure. Before stopping, close completed work or update both attention fields.
Never call poomsae.delete or poomsae.purge without explicit user authorization for
that exact action and issue. Use returned web_url values when linking issues.
`
