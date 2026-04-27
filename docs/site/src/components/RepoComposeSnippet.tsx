import CodeBlock from '@theme/CodeBlock';

import rootDockerCompose from '../generated/rootDockerCompose';

export default function RepoComposeSnippet() {
  return <CodeBlock language="yaml">{rootDockerCompose}</CodeBlock>;
}
