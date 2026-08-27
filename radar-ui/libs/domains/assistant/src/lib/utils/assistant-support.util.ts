import { AssistantSupportRequest } from '../interfaces';

/**
 * The fence the assistant wraps a finished support request in. It is the
 * contract with the prompt in notifier/pkg/assistant: the answer carries the
 * request as a code block marked this way, and the interface lifts it out to
 * offer an email.
 */
const SUPPORT_REQUEST_FENCE = 'support-request';

const SUPPORT_REQUEST_PATTERN = new RegExp('```' + SUPPORT_REQUEST_FENCE + '\\s*\\n([\\s\\S]*?)```', 'i');

const SUBJECT_PREFIX = /^subject:\s*/i;

/**
 * Pulls the finished support request out of an answer. The block is removed
 * from the text that gets rendered, so the user reads the request once, as a
 * card they can send, rather than twice.
 *
 * An answer without such a block — every turn of the interview before the last
 * one — comes back unchanged.
 */
export function extractSupportRequest(content: string): {
    content: string;
    request?: AssistantSupportRequest;
} {
    const match = SUPPORT_REQUEST_PATTERN.exec(content);

    if (!match) {
        return { content };
    }

    const lines = match[1].split('\n');
    const subjectLine = lines.findIndex((line) => SUBJECT_PREFIX.test(line.trim()));

    if (subjectLine < 0) {
        return { content };
    }

    const subject = lines[subjectLine].trim().replace(SUBJECT_PREFIX, '');
    const body = lines
        .slice(subjectLine + 1)
        .join('\n')
        .trim();

    return {
        content: content.replace(match[0], '').trim(),
        request: { subject, body }
    };
}

/**
 * Builds the mailto link the "Send by email" button opens. The subject and body
 * are encoded, so a request that quotes a path or an error message survives the
 * trip into the mail client intact.
 */
export function supportMailtoLink(address: string, request: AssistantSupportRequest): string {
    const query = `subject=${encodeURIComponent(request.subject)}&body=${encodeURIComponent(request.body)}`;

    return `mailto:${encodeURIComponent(address)}?${query}`;
}
