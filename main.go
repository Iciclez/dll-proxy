package main

import (
    "fmt"
	"flag"
	"os"
	"strings"
	"sync"
	"path/filepath"
	"io/ioutil"

	"github.com/iciclez/dll-proxy/win32"
)

func create_org_data_file(name string, file_name string, input_dll_file string) string {

	var b strings.Builder

	dll, err := ioutil.ReadFile(input_dll_file)
    if err != nil {
		fmt.Println(err)
		return ""
	}

	fmt.Fprintf(&b, "#pragma once\n\n")
	fmt.Fprintf(&b, "unsigned char %s_org_data[] = {", name)

	for index, byte_at_index := range dll {
		fmt.Fprintf(&b, "0x%.02x", byte_at_index)

		if index != len(dll) - 1 {
			fmt.Fprintf(&b, ", ")
		}
	}

	fmt.Fprintf(&b, "};")

	return b.String()
}

func create_dllmain_file(name string, file_name string, dll_exports [] win32.ModuleExportResult) string {

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

func create_hpp_file(name string, file_name string, dll_exports [] win32.ModuleExportResult) string {

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

func create_cpp_file(name string, file_name string, dll_exports [] win32.ModuleExportResult, pack_original_dll bool) string {

	var b strings.Builder

	/* include files */
	fmt.Fprintf(&b, "#include \"%s\"\n", name + ".hpp")
	fmt.Fprintf(&b, "#include <windows.h>\n")
	fmt.Fprintf(&b, "#include <strsafe.h>\n")

	if pack_original_dll {
		fmt.Fprintf(&b, "#include \"%s_org_data.hpp\"\n", name)

	} else {
		fmt.Fprintf(&b, "#include <shlobj.h>\n")

	}

	
	fmt.Fprintf(&b, "\n")

	/**/
	fmt.Fprintf(&b, "const size_t %s_size = %d;\n", name, len(dll_exports))
	fmt.Fprintf(&b,	"static FARPROC %s_functions[%s_size];\n", name, name)


	fmt.Fprintf(&b, "\n")
	/*asm codecave */
	for _, export := range dll_exports {
		fmt.Fprintf(&b, "/* [%.016X] %s:%d */\n", export.Code, name, export.Ordinal)
		fmt.Fprintf(&b, "__declspec(naked) void %s_%s()\n{\n\t__asm jmp dword ptr [%s_functions + %d]\n}\n\n", name, export.Name, name, (export.Ordinal - 1) * 4)
	}

	fmt.Fprintf(&b, "namespace %s\n{\n\tbool %s_initialize::initialize()\n\t{\n\t\tchar %s_path[MAX_PATH];\n\n", name, name, name)

	if pack_original_dll {

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
	
	for _, export := range dll_exports {
		fmt.Fprintf(&b, "\t\t%s_functions[%d] = GetProcAddress(%s_module, \"%s\");\n", name, export.Ordinal - 1, name, export.Name)
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

func create_def_file(name string, file_name string, dll_exports [] win32.ModuleExportResult) string {

	var b strings.Builder
	fmt.Fprintf(&b, "LIBRARY \"%s\"\n\n", name)
	fmt.Fprintf(&b, "EXPORTS\n")

	for _, export := range dll_exports {
		fmt.Fprintf(&b, "\t%s \t\t %s \t\t @%d \t PRIVATE\n", export.Name, "= " + name + "_" + export.Name, export.Ordinal)
	}

	return b.String()
}

func create_dll_proxy(input_dll_file *string, output_directory *string, dll_main *bool, pack_original_dll *bool) {

	var wait_group sync.WaitGroup
	dll_exports := win32.GetModuleExports(input_dll_file)

	// should be .dll
	extension := filepath.Ext(*input_dll_file)
	file_name := filepath.Base(*input_dll_file)
	file_name = file_name[0:len(file_name) - len(extension)]

	if *pack_original_dll {

		wait_group.Add(1)

		go func(name string, file_name string, input_dll_file string) {
			defer wait_group.Done()

			f, err := os.Create(file_name)
			if err != nil {
				fmt.Println(err)
				return
			}

			defer f.Close()

			result := create_org_data_file(name, file_name, input_dll_file)

			_, err = f.WriteString(result)
			if err != nil {
				fmt.Println(err)
				return
			}    
		}(file_name, filepath.Join(*output_directory, file_name + "_org_data.hpp"), *input_dll_file)

	}

	if *dll_main {

		wait_group.Add(1)

		go func(name string, file_name string, dll_exports [] win32.ModuleExportResult) {
			defer wait_group.Done()

			f, err := os.Create(file_name)
			if err != nil {
				fmt.Println(err)
				return
			}

			defer f.Close()

			result := create_dllmain_file(name, file_name, dll_exports)

			_, err = f.WriteString(result)
			if err != nil {
				fmt.Println(err)
				return
			}    

		}(file_name, filepath.Join(*output_directory, "dllmain.cpp"), dll_exports)

	}

	wait_group.Add(3)

	go func(name string, file_name string, dll_exports [] win32.ModuleExportResult) {
		defer wait_group.Done()

		f, err := os.Create(file_name)
		if err != nil {
			fmt.Println(err)
			return
		}

		defer f.Close()

		result := create_hpp_file(name, file_name, dll_exports)

		_, err = f.WriteString(result)
		if err != nil {
			fmt.Println(err)
			return
		}    
	}(file_name, filepath.Join(*output_directory, file_name + ".hpp"), dll_exports)

	go func(name string, file_name string, dll_exports [] win32.ModuleExportResult, pack_original_dll bool) {
		defer wait_group.Done()

		f, err := os.Create(file_name)
		if err != nil {
			fmt.Println(err)
			return
		}

		defer f.Close()

		result := create_cpp_file(name, file_name, dll_exports, pack_original_dll)

		_, err = f.WriteString(result)
		if err != nil {
			fmt.Println(err)
			return
		}

	}(file_name, filepath.Join(*output_directory, file_name + ".cpp"), dll_exports, *pack_original_dll)

	go func(name string, file_name string, dll_exports [] win32.ModuleExportResult) {
		defer wait_group.Done()

		f, err := os.Create(file_name)
		if err != nil {
			fmt.Println(err)
			return
		}

		defer f.Close()

		result := create_def_file(name, file_name, dll_exports)

		_, err = f.WriteString(result)
		if err != nil {
			fmt.Println(err)
			return
		}   

	}(file_name, filepath.Join(*output_directory, file_name + ".def"), dll_exports)

	wait_group.Wait()	
}

func print_usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
}

func handle_flags() {

	i := flag.String("i", "", "[required] input dll file")
	o := flag.String("o", "", "[optional] output directory")

	p := flag.Bool("pack", false, "[optional] pack original dll (default: false)")
	m := flag.Bool("dll_main", false, "[optional] create dllmain.cpp file (default: false)")

	flag.Parse()
	
	/* check input flag */

	if len(*i) == 0 {
		fmt.Println("error: input dll file empty")
		print_usage()
		return
	}

	input_file_information, err := os.Stat(*i)
    if err != nil {
		fmt.Println(err)
        return
	}

	if !input_file_information.Mode().IsRegular() || !strings.HasSuffix(*i, ".dll") {
		fmt.Println("error: incorrect flag types")
		print_usage()
		return 
	}

	/* check output flag */

	if len(*o) == 0 {
		current_directory, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			return
		}
		
		output_directory := filepath.Join(current_directory, *i)
		output_directory = strings.TrimSuffix(output_directory, filepath.Ext(output_directory))

		if _, err := os.Stat(output_directory); os.IsNotExist(err) {
			os.Mkdir(output_directory, os.ModeDir)
		}
		
		create_dll_proxy(i, &output_directory, m, p)
		return
	}

	output_directory_information, err := os.Stat(*o)
	if err != nil {
		fmt.Println(err)
        return
	}

	if !output_directory_information.Mode().IsDir() {
		fmt.Println("error: incorrect flag types")
		print_usage()
		return
	}

    create_dll_proxy(i, o, m, p)
}

func main() {
	handle_flags()
}