<script lang="ts">
  import StatusPage from './shared/StatusPage.svelte'

  interface Props {
    issueRef: string | undefined
  }

  let { issueRef }: Props = $props()
</script>

<StatusPage title="Open Poomsae from a terminal" description="This tab isn't signed in. Run:">
  {#snippet detail()}
    <code>poomsae ui{issueRef ? ` ${issueRef}` : ''}</code>
  {/snippet}
</StatusPage>
