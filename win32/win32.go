package win32

/*
#include <windows.h>
#include <stdint.h>
#include <stdio.h>

typedef struct {
	uint64_t nOrdinal;
	uint64_t nCode;
	char* pszName;
} module_export_result_t;

static inline PBYTE RvaAdjust(PIMAGE_DOS_HEADER pDosHeader, _In_ DWORD raddr)
{
	if (raddr != 0)
	{
		return ((PBYTE) pDosHeader) + raddr;
	}

	return NULL;
}

uint64_t __stdcall get_module_export_size(_In_ HMODULE hModule)
{
	PIMAGE_DOS_HEADER pDosHeader = (PIMAGE_DOS_HEADER) hModule;
	if (hModule == NULL)
	{
		pDosHeader = (PIMAGE_DOS_HEADER) GetModuleHandleW(NULL);
	}

	if (pDosHeader->e_magic != IMAGE_DOS_SIGNATURE)
	{
		SetLastError(ERROR_BAD_EXE_FORMAT);
		return 0;
	}

	PIMAGE_NT_HEADERS pNtHeader = (PIMAGE_NT_HEADERS)((PBYTE) pDosHeader + pDosHeader->e_lfanew);
	if (pNtHeader->Signature != IMAGE_NT_SIGNATURE)
	{
		SetLastError(ERROR_INVALID_EXE_SIGNATURE);
		return 0;
	}

	if (pNtHeader->FileHeader.SizeOfOptionalHeader == 0)
	{
		SetLastError(ERROR_EXE_MARKED_INVALID);
		return 0;
	}

	PIMAGE_EXPORT_DIRECTORY pExportDir = (PIMAGE_EXPORT_DIRECTORY)RvaAdjust(pDosHeader, pNtHeader->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_EXPORT].VirtualAddress);
	if (pExportDir == NULL)
	{
		SetLastError(ERROR_EXE_MARKED_INVALID);
		return 0;
	}

	return pExportDir->NumberOfFunctions;
}

BOOL __stdcall get_module_exports(_In_ HMODULE hModule, module_export_result_t *result, uint64_t result_size)
{
	PIMAGE_DOS_HEADER pDosHeader = (PIMAGE_DOS_HEADER) hModule;
	if (hModule == NULL)
	{
		pDosHeader = (PIMAGE_DOS_HEADER) GetModuleHandleW(NULL);
	}

	if (pDosHeader->e_magic != IMAGE_DOS_SIGNATURE)
	{
		SetLastError(ERROR_BAD_EXE_FORMAT);
		return FALSE;
	}

	PIMAGE_NT_HEADERS pNtHeader = (PIMAGE_NT_HEADERS)((PBYTE) pDosHeader + pDosHeader->e_lfanew);
	if (pNtHeader->Signature != IMAGE_NT_SIGNATURE)
	{
		SetLastError(ERROR_INVALID_EXE_SIGNATURE);
		return FALSE;
	}

	if (pNtHeader->FileHeader.SizeOfOptionalHeader == 0)
	{
		SetLastError(ERROR_EXE_MARKED_INVALID);
		return FALSE;
	}

	PIMAGE_EXPORT_DIRECTORY pExportDir = (PIMAGE_EXPORT_DIRECTORY)RvaAdjust(pDosHeader, pNtHeader->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_EXPORT].VirtualAddress);
	if (pExportDir == NULL)
	{
		SetLastError(ERROR_EXE_MARKED_INVALID);
		return FALSE;
	}

	if (result_size != pExportDir->NumberOfFunctions)
	{
		return FALSE;
	}

	PBYTE pExportDirEnd = (PBYTE) pExportDir + pNtHeader->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_EXPORT].Size;
	PDWORD pdwFunctions = (PDWORD) RvaAdjust(pDosHeader, pExportDir->AddressOfFunctions);
	PDWORD pdwNames = (PDWORD) RvaAdjust(pDosHeader, pExportDir->AddressOfNames);
	PWORD pwOrdinals = (PWORD) RvaAdjust(pDosHeader, pExportDir->AddressOfNameOrdinals);

	for (DWORD nFunc = 0; nFunc < pExportDir->NumberOfFunctions; nFunc++)
	{
		PBYTE pbCode = (pdwFunctions != NULL) ? (PBYTE) RvaAdjust(pDosHeader, pdwFunctions[nFunc]) : NULL;
		PCHAR pszName = NULL;

		// if the pointer is in the export region, then it is a forwarder.
		if (pbCode > (PBYTE) pExportDir && pbCode < pExportDirEnd)
		{
			pbCode = NULL;
		}

		for (DWORD n = 0; n < pExportDir->NumberOfNames; n++)
		{
			if (pwOrdinals[n] == nFunc)
			{
				pszName = (pdwNames != NULL) ?
					(PCHAR) RvaAdjust(pDosHeader, pdwNames[n]) : NULL;
				break;
			}
		}

		ULONG nOrdinal = pExportDir->Base + nFunc;

		printf("    %7d      %p %-30s\n", nOrdinal, pbCode, pszName ? pszName : "[NONAME]");

		result[nFunc].nOrdinal = nOrdinal;
		result[nFunc].nCode = (uint64_t)pbCode;
		result[nFunc].pszName = pszName;
	}

	SetLastError(NO_ERROR);
	return TRUE;

}
*/
import "C"
import "unsafe"

// CModuleExportResult : c struct representation for an export object
type CModuleExportResult struct {
	Ordinal C.uint64_t
	Code    C.uint64_t
	Name    *C.char
}

// ModuleExportResult : go struct representation for an export object
type ModuleExportResult struct {
	Ordinal uint64
	Code    uint64
	Name    string
}

// GetModuleExports : returns the the exports for a win32 binary
func GetModuleExports(executableFile *string) []ModuleExportResult {
	executableFileCString := C.CString(*executableFile)
	handle := C.LoadLibraryA(executableFileCString)

	defer C.free(unsafe.Pointer(executableFileCString))

	moduleExportSize := C.get_module_export_size(handle)
	moduleExportResults := make([]CModuleExportResult, int(moduleExportSize))
	C.get_module_exports(handle, (*C.module_export_result_t)(unsafe.Pointer(&moduleExportResults[0])), moduleExportSize)

	result := make([]ModuleExportResult, int(moduleExportSize))
	for i := 0; i < int(moduleExportSize); i++ {
		object := moduleExportResults[C.uint(i)]
		result[i] = ModuleExportResult{Ordinal: uint64(object.Ordinal), Code: uint64(object.Code), Name: C.GoString(object.Name)}
	}

	return result
}
