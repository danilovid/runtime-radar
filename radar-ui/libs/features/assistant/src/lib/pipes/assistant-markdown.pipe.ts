import { marked } from 'marked';
import { Pipe, PipeTransform } from '@angular/core';

/**
 * Renders the assistant's answer, which is Markdown by instruction.
 *
 * The result is bound with [innerHTML] and deliberately NOT passed through
 * DomSanitizer.bypassSecurityTrust*: the text is model output about untrusted
 * telemetry, so Angular's own sanitizer has to keep stripping scripts, event
 * handlers and anything else an event could have carried into it.
 */
@Pipe({
    name: 'assistantMarkdown'
})
export class AssistantFeatureMarkdownPipe implements PipeTransform {
    transform(value: string): string {
        if (!value) {
            return '';
        }

        return marked.parse(value, { async: false, gfm: true, breaks: true });
    }
}
