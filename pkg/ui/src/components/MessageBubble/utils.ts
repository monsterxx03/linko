export function formatTime(ts?: number): string {
  if (!ts) return '';
  return new Date(ts).toLocaleTimeString();
}

export function isEmptyContent(content: string | string[]): boolean {
  if (Array.isArray(content)) {
    return content.length === 0 || content.every(c => !c.trim());
  }
  return !content.trim();
}

export function decodeForTagCheck(content: string): string {
  return content
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&#39;/gi, "'")
    .replace(/&amp;/gi, '&')
    .replace(/\\u003c/g, '<')
    .replace(/\\u003e/g, '>');
}

export function hasSystemReminderTag(content: string): boolean {
  const decoded = decodeForTagCheck(content);
  const openMatch = /^<system-reminder[\s>]/i.test(decoded);
  if (!openMatch) return false;

  const closeRegex = /<\/system-reminder>$/i;
  return closeRegex.test(decoded);
}

export function getLastTwoPathSegments(path: string): string {
  const segments = path.split(/[\\/]/);
  if (segments.length <= 2) return path;
  return segments.slice(-2).join('/');
}

export interface ToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
}

const TOOL_FORMATTERS: Record<string, (args: Record<string, unknown>) => string> = {
  Read: (args) => args.file_path ? `Read ${getLastTwoPathSegments(String(args.file_path))}` : 'Read',
  Grep: (args) => args.pattern ? `Grep "${args.pattern}"` : 'Grep',
  Glob: (args) => args.pattern ? `Glob ${args.pattern}` : 'Glob',
  Edit: (args) => args.file_path ? `Edit ${getLastTwoPathSegments(String(args.file_path))}` : 'Edit',
  Write: (args) => args.file_path ? `Write ${getLastTwoPathSegments(String(args.file_path))}` : 'Write',
  Bash: (args) => {
    const cmd = args.command;
    if (!cmd) return 'Bash';
    const cmdStr = typeof cmd === 'string' ? cmd : (cmd as string[]).join(' ');
    return `Bash ${cmdStr.substring(0, 40)}${cmdStr.length > 40 ? '...' : ''}`;
  },
  TaskCreate: (args) => args.subject ? `TaskCreate ${args.subject}` : 'TaskCreate',
  TaskGet: (args) => args.taskId ? `TaskGet ${args.taskId}` : 'TaskGet',
  TaskUpdate: (args) => args.taskId ? `TaskUpdate ${args.taskId}` : 'TaskUpdate',
  Task: (args) => {
    const subagent = args.subagent_type;
    const desc = args.description;
    if (subagent && desc) return `Task ${subagent}: ${desc}`;
    if (subagent) return `Task ${subagent}`;
    if (desc) return `Task ${desc}`;
    return 'Task';
  },
  Skill: (args) => {
    const skill = args.skill;
    const argsVal = args.args;
    let argsStr = '';
    if (argsVal !== undefined && argsVal !== null) {
      argsStr = typeof argsVal === 'string' ? argsVal : String(argsVal);
    }
    if (skill && argsStr) {
      const truncatedArgs = argsStr.length > 40 ? argsStr.substring(0, 40) + '...' : argsStr;
      return `Skill ${skill}: ${truncatedArgs}`;
    }
    if (skill) return `Skill ${skill}`;
    if (argsStr) {
      const truncatedArgs = argsStr.length > 40 ? argsStr.substring(0, 40) + '...' : argsStr;
      return `Skill ${truncatedArgs}`;
    }
    return 'Skill';
  },
};

export function formatToolCall(call: { function: { name: string; arguments: string } }): string {
  const { name, arguments: argsStr } = call.function;

  try {
    const args = JSON.parse(argsStr);
    const formatter = TOOL_FORMATTERS[name];
    return formatter ? formatter(args) : name;
  } catch {
    return name;
  }
}
