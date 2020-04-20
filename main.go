package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/iciclez/dll-proxy/win32"
)

func createOriginalDataFile(name string, fileName string, inputDllFile string) string {

	var b strings.Builder

	dll, err := ioutil.ReadFile(inputDllFile)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	fmt.Fprintf(&b, "#pragma once\n\n")
	fmt.Fprintf(&b, "unsigned char %s_org_data[] = {", name)

	for index, byteAtIndex := range dll {
		fmt.Fprintf(&b, "0x%.02x", byteAtIndex)

		if index != len(dll)-1 {
			fmt.Fprintf(&b, ", ")
		}
	}

	fmt.Fprintf(&b, "};")

	return b.String()
}

func createDllMainFile(name string, fileName string, dllExports []win32.ModuleExportResult) string {

	var b strings.Builder

	fmt.Fprintf(&b, `#include <windows.h>
#include "%s.hpp"
	
BOOL WINAPI DllMain(_In_ HINSTANCE hinstDLL, _In_ DWORD fdwReason, _In_ LPVOID lpvReserved)
{
	UNREFERENCED_PARAMETER(lpvReserved);
	
	switch (fdwReason)
	{
	case DLL_PROCESS_ATTACH:
		DisableThreadLibraryCalls(hinstDLL);
		if (%s::%s_initialize::initialize())
		{
			//
		}
		break;
	
	case DLL_PROCESS_DETACH:
		break;
	}
	
	return TRUE;
}
	`, name, name, name)

	return b.String()
}

func createHeaderFile(name string, fileName string, dllExports []win32.ModuleExportResult) string {

	var b strings.Builder
	fmt.Fprintf(&b, `#pragma once

namespace %s
{
	class %s_initialize
	{
	public:
		static bool initialize();
	};
}`, name, name)

	return b.String()
}

func createSourceFile(name string, fileName string, dllExports []win32.ModuleExportResult, packOriginalDll bool) string {

	var b strings.Builder

	/* include files */
	fmt.Fprintf(&b, "#include \"%s\"\n", name+".hpp")
	fmt.Fprintf(&b, "#include <windows.h>\n")
	fmt.Fprintf(&b, "#include <strsafe.h>\n")

	if packOriginalDll {
		fmt.Fprintf(&b, "#include \"%s_org_data.hpp\"\n", name)
	} else {
		fmt.Fprintf(&b, "#include <shlobj.h>\n")
	}

	fmt.Fprintf(&b, "\n")

	/**/
	fmt.Fprintf(&b, "const size_t %s_size = %d;\n", name, len(dllExports))
	fmt.Fprintf(&b, "static FARPROC %s_functions[%s_size];\n", name, name)

	fmt.Fprintf(&b, "\n")
	/*asm codecave */
	for _, export := range dllExports {
		fmt.Fprintf(&b, "/* [%.016X] %s:%d */\n", export.Code, name, export.Ordinal)
		fmt.Fprintf(&b, "__declspec(naked) void %s_%s()\n{\n\t__asm jmp dword ptr [%s_functions + %d]\n}\n\n", name, export.Name, name, (export.Ordinal-1)*4)
	}

	fmt.Fprintf(&b, "namespace %s\n{\n\tbool %s_initialize::initialize()\n\t{\n\t\tchar %s_path[MAX_PATH];\n\n", name, name, name)

	if packOriginalDll {

		fmt.Fprintf(&b, `
		if (!SUCCEEDED(StringCchPrintf(%s_path, MAX_PATH, "%%s%%s", %s_path, "%s_data.dll")))
		{
			return false;
		}

		DWORD number_of_bytes_written = 0;
		HANDLE file_handle = CreateFile(%s_path, GENERIC_READ | GENERIC_WRITE, 0, 0, CREATE_ALWAYS, 0, 0);

		if (!file_handle)
		{
			return false;
		}
		
		WriteFile(file_handle, %s_path, sizeof(%s_org_data), &number_of_bytes_written, 0);
		CloseHandle(file_handle); 

`, name, name, name, name, name, name)

	} else {

		fmt.Fprintf(&b, `
		if (!SUCCEEDED(SHGetFolderPath(0, CSIDL_SYSTEM, 0, 0, %s_path)))
		{
			return false;
		}

		if (!SUCCEEDED(StringCchPrintf(%s_path, MAX_PATH, "%%s%%s", %s_path, "\\%s.dll")))
		{
			return false;
		}

`, name, name, name, name)

	}

	fmt.Fprintf(&b, "\t\tHMODULE %s_module = LoadLibrary(%s_path);\n", name, name)
	fmt.Fprintf(&b, "\t\tif (!%s_module)\n", name)
	fmt.Fprintf(&b, "\t\t{\n")
	fmt.Fprintf(&b, "\t\t\treturn false;\n")
	fmt.Fprintf(&b, "\t\t}\n\n")

	for _, export := range dllExports {
		fmt.Fprintf(&b, "\t\t%s_functions[%d] = GetProcAddress(%s_module, \"%s\");\n", name, export.Ordinal-1, name, export.Name)
	}

	fmt.Fprintf(&b, `
		for (int i = 0; i < %s_size; ++i)
		{
			if (!%s_functions[i])
			{
				return false;
			}
		}

		return true;
	`, name, name)

	fmt.Fprintf(&b, "\n\t}\n}\n")

	return b.String()
}

func createDefinitionFile(name string, fileName string, dllExports []win32.ModuleExportResult) string {

	var b strings.Builder
	fmt.Fprintf(&b, "LIBRARY \"%s\"\n\n", name)
	fmt.Fprintf(&b, "EXPORTS\n")

	for _, export := range dllExports {
		fmt.Fprintf(&b, "\t%s \t\t %s \t\t @%d \t PRIVATE\n", export.Name, "= "+name+"_"+export.Name, export.Ordinal)
	}

	return b.String()
}

func createDllProxy(inputDllFile *string, outputDirectory *string, dllMain *bool, packOriginalDll *bool) {

	var waitGroup sync.WaitGroup
	dllExports := win32.GetModuleExports(inputDllFile)

	// should be .dll
	extension := filepath.Ext(*inputDllFile)
	fileName := filepath.Base(*inputDllFile)
	fileName = fileName[0 : len(fileName)-len(extension)]

	if *packOriginalDll {

		waitGroup.Add(1)

		go func(name string, fileName string, inputDllFile string) {
			defer waitGroup.Done()

			f, err := os.Create(fileName)
			if err != nil {
				fmt.Println(err)
				return
			}

			defer f.Close()

			result := createOriginalDataFile(name, fileName, inputDllFile)

			_, err = f.WriteString(result)
			if err != nil {
				fmt.Println(err)
				return
			}
		}(fileName, filepath.Join(*outputDirectory, fileName+"_org_data.hpp"), *inputDllFile)

	}

	if *dllMain {

		waitGroup.Add(1)

		go func(name string, fileName string, dllExports []win32.ModuleExportResult) {
			defer waitGroup.Done()

			f, err := os.Create(fileName)
			if err != nil {
				fmt.Println(err)
				return
			}

			defer f.Close()

			result := createDllMainFile(name, fileName, dllExports)

			_, err = f.WriteString(result)
			if err != nil {
				fmt.Println(err)
				return
			}

		}(fileName, filepath.Join(*outputDirectory, "dllmain.cpp"), dllExports)

	}

	waitGroup.Add(3)

	go func(name string, fileName string, dllExports []win32.ModuleExportResult) {
		defer waitGroup.Done()

		f, err := os.Create(fileName)
		if err != nil {
			fmt.Println(err)
			return
		}

		defer f.Close()

		result := createHeaderFile(name, fileName, dllExports)

		_, err = f.WriteString(result)
		if err != nil {
			fmt.Println(err)
			return
		}
	}(fileName, filepath.Join(*outputDirectory, fileName+".hpp"), dllExports)

	go func(name string, fileName string, dllExports []win32.ModuleExportResult, packOriginalDll bool) {
		defer waitGroup.Done()

		f, err := os.Create(fileName)
		if err != nil {
			fmt.Println(err)
			return
		}

		defer f.Close()

		result := createSourceFile(name, fileName, dllExports, packOriginalDll)

		_, err = f.WriteString(result)
		if err != nil {
			fmt.Println(err)
			return
		}

	}(fileName, filepath.Join(*outputDirectory, fileName+".cpp"), dllExports, *packOriginalDll)

	go func(name string, fileName string, dllExports []win32.ModuleExportResult) {
		defer waitGroup.Done()

		f, err := os.Create(fileName)
		if err != nil {
			fmt.Println(err)
			return
		}

		defer f.Close()

		result := createDefinitionFile(name, fileName, dllExports)

		_, err = f.WriteString(result)
		if err != nil {
			fmt.Println(err)
			return
		}

	}(fileName, filepath.Join(*outputDirectory, fileName+".def"), dllExports)

	waitGroup.Wait()
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
}

func handleFlags() {

	i := flag.String("i", "", "[required] input dll file")
	o := flag.String("o", "", "[optional] output directory")

	p := flag.Bool("pack", false, "[optional] pack original dll (default: false)")
	m := flag.Bool("dll_main", false, "[optional] create dllmain.cpp file (default: false)")

	flag.Parse()

	/* check input flag */

	if len(*i) == 0 {
		fmt.Println("error: input dll file empty")
		printUsage()
		return
	}

	inputFileInformation, err := os.Stat(*i)
	if err != nil {
		fmt.Println(err)
		return
	}

	if !inputFileInformation.Mode().IsRegular() || !strings.HasSuffix(*i, ".dll") {
		fmt.Println("error: incorrect flag types")
		printUsage()
		return
	}

	/* check output flag */

	if len(*o) == 0 {
		currentDirectory, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			return
		}

		outputDirectory := filepath.Join(currentDirectory, *i)
		outputDirectory = strings.TrimSuffix(outputDirectory, filepath.Ext(outputDirectory))

		if _, err := os.Stat(outputDirectory); os.IsNotExist(err) {
			os.Mkdir(outputDirectory, os.ModeDir)
		}

		createDllProxy(i, &outputDirectory, m, p)
		return
	}

	outputDirectoryInformation, err := os.Stat(*o)
	if err != nil {
		fmt.Println(err)
		return
	}

	if !outputDirectoryInformation.Mode().IsDir() {
		fmt.Println("error: incorrect flag types")
		printUsage()
		return
	}

	createDllProxy(i, o, m, p)
}

func main() {
	handleFlags()
}
