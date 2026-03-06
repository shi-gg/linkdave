export async function unwrap<T extends Promise<unknown>>(promise: T) {
    try {
        const result = await promise;
        return [result, null] as const;
    } catch (error) {
        return [null, error] as const;
    }
}

export const constructUri = {
    mp3: (url: `http://${string}` | `https://${string}`) => url,
    tts: (text: string, voice: string, translate: boolean = false) => `tts://invoke?text=${encodeURIComponent(text)}&speaker=${encodeURIComponent(voice)}&translate=${translate ? "true" : "false"}`
};