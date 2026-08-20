import { Injectable } from '@angular/core';

import { ASSISTANT_MAX_ATTACHMENT_BYTES } from '../constants/assistant.constant';
import { AssistantAttachment } from '../interfaces';

/**
 * Turns a picked file into text that travels inside the question.
 *
 * Nothing is uploaded: the file is read in the browser and appended to the
 * message, so there is no storage to secure and no new endpoint to authorise.
 * That also bounds what a file can do — it becomes data in a prompt, which the
 * assistant is told to treat as untrusted.
 */
@Injectable({
    providedIn: 'root'
})
export class AssistantAttachmentService {
    async read(file: File): Promise<AssistantAttachment> {
        const slice = file.slice(0, ASSISTANT_MAX_ATTACHMENT_BYTES);
        const content = await slice.text();

        return {
            name: file.name,
            size: file.size,
            content,
            isTruncated: file.size > ASSISTANT_MAX_ATTACHMENT_BYTES
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
}
