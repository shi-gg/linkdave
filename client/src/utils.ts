export async function unwrap<T extends Promise<unknown>>(promise: T) {
    try {
        const result = await promise;
        return [result, null] as const;
    } catch (error) {
        return [null, error] as const;
    }
}