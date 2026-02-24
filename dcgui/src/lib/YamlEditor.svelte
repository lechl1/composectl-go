<script>
  import { onMount } from "svelte";
  import { EditorView, basicSetup } from "codemirror";
  import { yaml } from "@codemirror/lang-yaml";
  import { oneDark } from "@codemirror/theme-one-dark";
  import {
    createEditor,
    playStack as playStackHandler,
    stopStack as stopStackHandler,
    deleteStack as deleteStackHandler,
    saveStack
  } from "$lib/stackManager.js";
  import {fetchStackDoc} from "./stackManager.js";
  import { logout } from "$lib/auth.js";
  function handleLogout() {
      logout();
  }
  let { selectedStack = "" } = $props();
  let doc = $state("");

  const id = `editor-${Math.random().toString(36).substr(2, 9)}`;
  const outputId = `output-${Math.random().toString(36).substr(2, 9)}`;
  let editorView = $state(null);
  let outputLog = $state(null);
  let showOutput = $state(false);
  let saveTimeout = $state(null);
  let isSaved = $state(false);
  let showEditor = $state(true);
  let outputStatus = $state(null); // 'success' | 'error' | null

  onMount(() => {
    // Create auto-save extension with keyup handler
    const autoSaveExtension = EditorView.domEventHandlers({
      keyup: () => {
        handleDocChange();
      }
    });

    editorView = createEditor({
      parent: document.getElementById(id),
      extensions: [basicSetup, oneDark, yaml(), autoSaveExtension],
      doc: doc || "",
    });

    outputLog = createEditor({
      parent: document.getElementById(outputId),
      extensions: [basicSetup, oneDark]
    });


    return () => {
      // Cleanup timeout on unmount
      if (saveTimeout) {
        clearTimeout(saveTimeout);
      }
    };
  });

  $effect(async () => {
      if (selectedStack) {
          appendOutput("", true)
          let result = null;
          try {
              result = await fetchStackDoc(selectedStack, appendOutput)
              showEditor = true
          } catch (error) {
              showOutput = true
              return;
          }
         if (result) {
           doc = result.text || result;
           if (showOutput) {
             outputStatus = result?.success ? 'success' : 'error';
           }
         }
      }
  });

  $effect(() => {
    if (editorView && doc) {
      editorView.dispatch({
        changes: {
          from: 0,
          to: editorView.state.doc.length,
          insert: doc,
        },
      });
    }
  });

  $effect(() => {
    if (showOutput && outputLog) {
      // Scroll to bottom when logs are opened
      const docLength = outputLog.state.doc.length;
      outputLog.dispatch({
        selection: { anchor: docLength },
        scrollIntoView: true
      });
    }
  });

  $effect(() => {
    if (outputStatus && outputLog) {
      // Scroll to bottom when operation completes (success or error)
      const docLength = outputLog.state.doc.length;
      outputLog.dispatch({
        selection: { anchor: docLength },
        scrollIntoView: true
      });
    }
  });

  function handleDocChange() {
    // Clear previous timeout
    if (saveTimeout) {
      clearTimeout(saveTimeout);
    }

    // Reset saved state when user types
    isSaved = false;

    // Set new timeout to save after 1 second of inactivity
    saveTimeout = setTimeout(async () => {
      if (selectedStack && editorView) {
        const content = editorView.state.doc.toString();
        const result = await saveStack(selectedStack, content);
        isSaved = result.success;
      }
    }, 1000);
  }

  // Helper function to append output to the CodeMirror log
  function appendOutput(text, clear = false) {
    if (!outputLog) return;

    if (clear) {
      outputLog.dispatch({
        changes: {
          from: 0,
          to: outputLog.state.doc.length,
          insert: "",
        },
      });
    } else {
      outputLog.dispatch({
        changes: {
          from: outputLog.state.doc.length,
          insert: text,
        },
      });
    }
  }

  async function playStack() {
    if (selectedStack && editorView && outputLog) {
      appendOutput("", true)
      showOutput = true;
      outputStatus = null;
      const docContent = editorView.state.doc.toString();
      const result = await playStackHandler(selectedStack, docContent, appendOutput)
      outputStatus = result?.success ? 'success' : 'error';
    }
  }

  async function stopStack() {
    if (selectedStack && outputLog) {
      appendOutput("", true)
      showOutput = true;
      outputStatus = null;
      const docContent = editorView.state.doc.toString();
      const result = await stopStackHandler(selectedStack, docContent, appendOutput)
      outputStatus = result?.success ? 'success' : 'error';
    }
  }

  async function deleteStack() {
    if (selectedStack && outputLog) {
      appendOutput("", true)
      showOutput = true
      outputStatus = null
      const result = await deleteStackHandler(selectedStack, appendOutput)
      outputStatus = result?.success ? 'success' : 'error';
    }
  }

  function toggleEditor() {
    showEditor = !showEditor;
  }

  function toggleLogs() {
    showOutput = !showOutput;
  }
</script>


<div class="flex flex-col w-full h-full overflow-hidden gap-1">
    <div class="flex justify-between">
        <div class="flex gap-1">
            <button class="cursor-pointer p-2 border rounded border-white/30 text-white/80 text-sm" onclick={playStack}>🚀 Deploy</button>
            <button class="cursor-pointer p-2 border rounded border-white/30 text-white/80 text-sm" onclick={stopStack}>🛑 Stop</button>
            <button class="cursor-pointer p-2 border rounded border-white/30 text-white/80 text-sm" onclick={deleteStack}>🗑️ Trash</button>
            <button class="cursor-pointer p-2 border rounded text-white/80 text-sm {showEditor ? 'border-blue-500 bg-blue-500/20' : 'border-white/30'}" onclick={toggleEditor}>✏️ Edit</button>
            <button class="cursor-pointeffr p-2 border rounded text-white/80 text-sm {showOutput ? 'border-blue-500 bg-blue-500/20' : 'border-white/30'}" onclick={toggleLogs}>📋 Logs</button>
        </div>
        <div class="flex gap-1">
            <button onclick={handleLogout} class="text-white/80 rounded border-1 border-red-500/50 p-2 cursor-pointer bg-red-500/20 hover:bg-red-500/30 transition-colors">
                🚪 Logout
            </button>
        </div>
    </div>
    <div class="flex-1 flex flex-col gap-1 overflow-hidden">
        <div id={id} class="overflow-auto border rounded {isSaved ? 'border-green-500' : 'border-white/20'} {showEditor ? (showOutput ? 'flex-[7]' : 'flex-1') : 'hidden'}"></div>
        <div id={outputId} class="overflow-auto border rounded {outputStatus === 'success' ? 'border-green-500' : outputStatus === 'error' ? 'border-red-500' : 'border-white/20'} {showOutput ? 'flex-[3]' : 'hidden'}"></div>
    </div>
</div>
