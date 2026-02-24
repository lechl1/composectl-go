import { EditorView, basicSetup } from "codemirror";
import { authFetch } from "./auth.js";

// Common function to handle streaming responses
async function handleStreamingResponse(response, log, successMessage, errorPrefix) {
  if (response.ok) {
    const decoder = new TextDecoder();
    await response.body.pipeTo(new WritableStream({
      write(chunk) {
        const text = decoder.decode(chunk, { stream: true });
        log(text);
      },
      close() {

      },
      abort(err) {
        throw `Stream aborted: ${err}`
      }
    }));
  } else {
     throw "Response error: " + errorPrefix + ": " + response.statusText;
  }
}

export function createEditor({ parent, mode, doc = "", extensions = [] }) {
  var editorView = new EditorView({
    parent,
    mode,
    doc,
    extensions: [
      basicSetup,
      ...extensions,
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          const value = update.state.doc.toString();
        }
      }),
    ]
  });
  return editorView;
}

// Helper function to calculate stack state from containers
function calculateStackState(stack) {
  const containers = stack.containers || [];
  if (containers.length === 0) return 'unknown';

  const runningCount = containers.filter(c => {
    // running is now a boolean nested in state object
    const state = c.state;
    if (!state) return false;
    return state.running === true;
  }).length;

  if (runningCount === containers.length) return 'running';
  if (runningCount === 0) return 'stopped';
  return 'partial';
}


export function getStackStatusEmoji(stack) {
  switch (calculateStackState(stack)) {
    case 'running':
      return '🟢';
    case 'partial':
      return '🟡';
    case 'stopped':
      return '🔴';
    default:
      return '⚪';
  }
}

export function getContainerCounts(stack) {
  const containers = stack.containers || [];
  const total = containers.length;
  const running = containers.filter(c => {
    const state = c.state;
    if (!state) return false;
    return state.running === true;
  }).length;
  return { running, total };
}

async function ftch({ url, log, successMessage, errorMessage, ...options }) {
  return await authFetch(url, options)
      .then(async response => {
        const responseText = [];
        if (response.ok) {
          if (log) {
            const decoder = new TextDecoder();
            await response.body.pipeTo(new WritableStream({
              write(chunk) {
                const text = decoder.decode(chunk, {stream: true});
                log(text);
                responseText.push(text);
              },
              close() {

              },
              abort(err) {
                throw `Stream aborted: ${err}`
              }
            }));
            return { text: responseText.join(''), success: true };
          } else {
            return { text: await response.text(), success: true };
          }
        } else {
          throw response.statusText;
        }
      })
      .then(result => {
        log(`\n${result.text}`);
        log(`\n✅ ${successMessage}`);
        return { text: result.text, success: true };
      })
      .catch(responseText => {
        log(`\n${responseText}`);
        log(`\n❌ ${errorMessage}`);
        return { text: responseText, success: false };
      });
}

export async function get({ url, log, successMessage, errorMessage, ...options }) {
  return await ftch({
    url,
    log,
    successMessage,
    errorMessage,
    ...options,
    method: 'GET'
  });
}

export async function put({ url, log, successMessage, errorMessage, ...options }) {
    return await ftch({
      url,
      log,
      successMessage,
      errorMessage,
      ...options,
      method: 'PUT'
    });
}

export async function del({ url, log, successMessage, errorMessage, ...options }) {
  return await ftch({
    url,
    log,
    successMessage,
    errorMessage,
    ...options,
    method: 'DELETE'
  });
}

export async function fetchStacks() {
    const response = await authFetch('/api/stacks');
    if (!response.ok) {
      return []
    }
    return (await response.json() || []).sort((a, b) => a.name.localeCompare(b.name));
}


export async function fetchStackDoc(stackName, log) {
  return await get({
    url: `/api/stacks/${stackName}`,
    log,
    successMessage: 'Stack content fetched successfully',
    errorMessage: 'Failed to fetch stack content'
  });
}

export async function playStack(stackName, body, log) {
  return await put({
    url: `/api/stacks/${stackName}/start`,
    body,
    log,
    successMessage: 'Stack deployed successfully',
    errorMessage: 'Failed to deploy stack'
  })
}

export async function stopStack(stackName, body, log) {
  return await put({
    url: `/api/stacks/${stackName}/stop`,
    body,
    log,
    successMessage: 'Stack stopped successfully',
    errorMessage: 'Failed to stop stack'
  })
}

export async function deleteStack(stackName, body, log) {
  return await del({
    url: `/api/stacks/${stackName}`,
    body,
    log,
    successMessage: 'Stack deleted successfully',
    errorMessage: 'Failed to delete stack'
  })
}

export async function saveStack(stackName, body, log) {
  return await put({
    url: `/api/stacks/${stackName}`,
    body,
    log,
    successMessage: 'Stack saved successfully',
    errorMessage: 'Failed to save stack'
  })
}

