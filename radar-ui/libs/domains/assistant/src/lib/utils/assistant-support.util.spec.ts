import { extractSupportRequest, supportMailtoLink } from './assistant-support.util';

describe('extractSupportRequest', () => {
    it('lifts the request out of the answer', () => {
        const answer = [
            'Thanks, that is enough to write it up.',
            '',
            '```support-request',
            'Subject: Events stop arriving after a node restart',
            '',
            'What happens: the events page is empty for node-3.',
            'Expected: events as before the restart.',
            '```',
            ''
        ].join('\n');

        const { content, request } = extractSupportRequest(answer);

        expect(request?.subject).toBe('Events stop arriving after a node restart');
        expect(request?.body).toContain('the events page is empty for node-3');
        // The block is rendered as a card, so it is taken out of the prose.
        expect(content).toBe('Thanks, that is enough to write it up.');
    });

    it('leaves an answer without a request alone', () => {
        const answer = 'Which namespace does this happen in?';

        expect(extractSupportRequest(answer)).toEqual({ content: answer });
    });

    it('ignores a block that carries no subject', () => {
        const answer = ['```support-request', 'no subject here', '```'].join('\n');

        expect(extractSupportRequest(answer).request).toBeUndefined();
    });
});

describe('supportMailtoLink', () => {
    it('encodes what the mail client would otherwise cut', () => {
        const link = supportMailtoLink('support@example.com', {
            subject: 'Rule & scope',
            body: 'Path: /usr/bin/curl?x=1\nSecond line'
        });

        expect(link.startsWith('mailto:support%40example.com?')).toBe(true);
        expect(link).toContain('subject=Rule%20%26%20scope');
        expect(link).toContain('%2Fusr%2Fbin%2Fcurl%3Fx%3D1');
        expect(link).toContain('%0A');
    });
});
