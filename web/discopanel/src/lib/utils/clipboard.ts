/**
 * Copy text to clipboard with fallback for non-secure contexts (HTTP).
 * navigator.clipboard only works in secure contexts (HTTPS or localhost).
 */
export async function copyToClipboard(text: string): Promise<boolean> {
	if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch {
			// Fallback if clipboard API throws
		}
	}

	try {
		const textArea = document.createElement('textarea');
		textArea.value = text;
		textArea.setAttribute('readonly', '');
		textArea.style.position = 'fixed';
		textArea.style.left = '-9999px';
		textArea.style.top = '-9999px';
		textArea.style.opacity = '0';
		document.body.appendChild(textArea);
		textArea.focus();
		textArea.select();
		const success = document.execCommand('copy');
		document.body.removeChild(textArea);
		return success;
	} catch {
		return false;
	}
}
