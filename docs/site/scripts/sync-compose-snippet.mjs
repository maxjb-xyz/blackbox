import {mkdir, readFile, writeFile} from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const repoRoot = path.resolve(__dirname, '../../..');
const sourcePath = path.join(repoRoot, 'docker-compose.yml');
const targetDir = path.join(__dirname, '../src/generated');
const targetPath = path.join(targetDir, 'rootDockerCompose.ts');

const compose = await readFile(sourcePath, 'utf8');

const output = `// Auto-generated from repo-root docker-compose.yml. Do not edit by hand.
const rootDockerCompose = ${JSON.stringify(compose)};

export default rootDockerCompose;
`;

await mkdir(targetDir, {recursive: true});
await writeFile(targetPath, output, 'utf8');
