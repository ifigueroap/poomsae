#!/usr/bin/env node
import {readFileSync,appendFileSync} from 'node:fs';
const args=process.argv.slice(2),state=JSON.parse(readFileSync(process.env.POOMSAE_PI_TEST_STATE,'utf8'));
appendFileSync(process.env.POOMSAE_PI_TEST_CALLS,JSON.stringify({executable:process.argv[1],args,cwd:process.cwd(),server:process.env.POOMSAE_SERVER,author:process.env.POOMSAE_AUTHOR,teammate:process.env.POOMSAE_TEAMMATE,recipient:process.env.POOMSAE_INBOX_USER})+'\n');
if(args[0]==='agent-contract-hook') {
 if(state.failContract)process.exit(1);
 process.stdout.write(JSON.stringify({hookSpecificOutput:{additionalContext:state.contract}}));
} else if(args[0]==='inbox') {
 if(state.failInbox)process.exit(1);
 process.stdout.write(state.inbox);
} else if(args[0]!=='agent-hook')process.exit(2);
