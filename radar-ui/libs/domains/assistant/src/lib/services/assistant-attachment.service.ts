import { Injectable } from '@angular/core';

import { ASSISTANT_MAX_ATTACHMENTS_BYTES } from '../constants/assistant.constant';
import { AssistantAttachment } from '../interfaces';

/**
 * Turns picked files into text that travels inside the question.
 *
 * Nothing is uploaded: a file is read in the browser and appended to the
 * message, so there is no storage to secure and no new endpoint to authorise.
 * That also bounds what a file can do — it becomes data in a prompt, which the
 * assistant is told to treat as untrusted.
 *
 * The files of one message share ASSISTANT_MAX_ATTACHMENTS_BYTES, because they
 * all end up in that one message and the server refuses a message over its own
 * limit. A file that does not fit what is left is attached truncated and says
 * so, both to the model and to the user.
 */
@Injectable({
    providedIn: 'root'
})
export class AssistantAttachmentService {
    /**
     * Reads picked files into what is left of the message's budget, in the
     * order they were picked.
     */
    async readWithinBudget(files: File[], attached: AssistantAttachment[]): Promise<AssistantAttachment[]> {
        let left = ASSISTANT_MAX_ATTACHMENTS_BYTES - this.usedBytes(attached);
        const read: AssistantAttachment[] = [];

        for (const file of files) {
            const attachment = await this.read(file, Math.max(left, 0));

            left -= this.byteLength(attachment.content);
            read.push(attachment);
        }

        return read;
    }

    /** Reads one file, up to budget bytes of it. */
    async read(file: File, budget: number): Promise<AssistantAttachment> {
        const slice = file.slice(0, budget);
        const content = budget > 0 ? await slice.text() : '';

        return {
            name: file.name,
            size: file.size,
            content,
            isTruncated: file.size > budget
        };
    }

    /**
     * Renders attachments into the message text. The delimiters tell the model
     * where the file starts and ends, and a file that quotes the closing one
     * cannot end the block early.
     */
    render(attachments: AssistantAttachment[]): string {
        return attachments
            .map((attachment) => {
                const content = attachment.content
                    .split('</attached_file>')
                    .join('[/attached_file]')
                    .split('<attached_file')
                    .join('[attached_file');
                const truncated = attachment.isTruncated ? ' (truncated)' : '';

                return `\n\n<attached_file name="${attachment.name}"${truncated}>\n${content}\n</attached_file>`;
            })
            .join('');
    }

    /** How much of the budget the attachments of a message already spend. */
    usedBytes(attachments: AssistantAttachment[]): number {
        return attachments.reduce((total, attachment) => total + this.byteLength(attachment.content), 0);
    }

    /**
     * The size of a string as the request will carry it. Counting characters
     * would undercount every Cyrillic message by half.
     */
    private byteLength(text: string): number {
        return new TextEncoder().encode(text).length;
    }
}
