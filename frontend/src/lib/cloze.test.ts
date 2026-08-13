import { describe, expect, it } from 'vitest';
import {
	detectNoteType,
	maskStateFor,
	nextClozeNumber,
	templateLabel,
	stripClozeMarkers,
	parseOcclusionFragment,
	wrapAsCloze,
	type Occlusion,
	type OcclusionFragment,
} from './cloze';

const masks: Occlusion = {
	mode: 'hide-one',
	rects: [{ id: 1, x: 0.1, y: 0.1, w: 0.2, h: 0.1 }],
};

describe('detectNoteType', () => {
	it('is basic for plain text', () => {
		expect(detectNoteType('just words', null)).toBe('basic');
	});

	it('is cloze when the front carries a deletion', () => {
		expect(detectNoteType('the {{c1::answer}}', null)).toBe('cloze');
	});

	it('ignores malformed markers', () => {
		expect(detectNoteType('{{c::x}} {{c0::y}}', null)).toBe('basic');
	});

	it('is image-cloze when masks exist', () => {
		expect(detectNoteType('![cow](/api/images/abc)', masks)).toBe('image-cloze');
	});

	it('masks win over text markers — one note, one type', () => {
		expect(detectNoteType('{{c1::x}} ![cow](/api/images/abc)', masks)).toBe('image-cloze');
	});

	it('empty mask set is not image-cloze', () => {
		expect(detectNoteType('![cow](/api/images/abc)', { mode: 'hide-one', rects: [] })).toBe(
			'basic',
		);
	});
});

describe('nextClozeNumber', () => {
	it('starts at 1', () => {
		expect(nextClozeNumber('no markers')).toBe(1);
	});

	it('goes one past the highest in use', () => {
		expect(nextClozeNumber('{{c1::a}} {{c3::b}}')).toBe(4);
	});
});

describe('wrapAsCloze', () => {
	it('wraps text as a numbered deletion', () => {
		expect(wrapAsCloze('Ottawa', 2)).toBe('{{c2::Ottawa}}');
	});
});

describe('stripClozeMarkers', () => {
	it('reduces markers to their answer text', () => {
		expect(stripClozeMarkers('{{c1::Ottawa}} is in {{c2::Canada::country}}.')).toBe(
			'Ottawa is in Canada.',
		);
	});

	it('leaves plain text alone', () => {
		expect(stripClozeMarkers('nothing here')).toBe('nothing here');
	});
});

describe('parseOcclusionFragment', () => {
	it('reads the payload the server embeds in an image URL', () => {
		const payload = {
			mode: 'hide-all',
			side: 'q',
			test: 2,
			rects: [{ id: 2, x: 0.5, y: 0.25, w: 0.2, h: 0.1 }],
		};
		const src = '/api/images/abc#occ=' + encodeURIComponent(JSON.stringify(payload));
		expect(parseOcclusionFragment(src)).toEqual(payload);
	});

	it('is null for an image without a fragment', () => {
		expect(parseOcclusionFragment('/api/images/abc')).toBeNull();
	});

	it('is null for a mangled payload rather than throwing mid-render', () => {
		expect(parseOcclusionFragment('/api/images/abc#occ=%7Bnot-json')).toBeNull();
	});
});

describe('templateLabel', () => {
	it('names every template a note can produce', () => {
		expect(templateLabel('forward')).toBe('Front → back');
		expect(templateLabel('reverse')).toBe('Back → front');
		expect(templateLabel('cloze:2')).toBe('Cloze 2');
		expect(templateLabel('occ:3')).toBe('Mask 3');
	});

	it('falls back to the raw template for anything unknown', () => {
		expect(templateLabel('sideways')).toBe('sideways');
	});
});

describe('maskStateFor', () => {
	const frag = (mode: string, side: string, test: number): OcclusionFragment => ({
		mode,
		side,
		test,
		rects: [],
	});

	it('hide-one question: only the tested mask is covered', () => {
		expect(maskStateFor(1, frag('hide-one', 'q', 1))).toBe('covered-tested');
		expect(maskStateFor(2, frag('hide-one', 'q', 1))).toBe('hidden');
	});

	it('hide-all question: everything covered, the tested mask marked', () => {
		expect(maskStateFor(1, frag('hide-all', 'q', 1))).toBe('covered-tested');
		expect(maskStateFor(2, frag('hide-all', 'q', 1))).toBe('covered');
	});

	it('answer: the tested mask is outlined, everything else revealed', () => {
		for (const mode of ['hide-one', 'hide-all']) {
			expect(maskStateFor(1, frag(mode, 'a', 1))).toBe('outline');
			expect(maskStateFor(2, frag(mode, 'a', 1))).toBe('hidden');
		}
	});
});
