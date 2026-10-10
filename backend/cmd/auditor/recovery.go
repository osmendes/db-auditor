package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mayconmendes-qc/db-auditor/internal/repository"
	"golang.org/x/term"
)

func recoverOperator(args []string) error {
	flags := flag.NewFlagSet("recover-operator", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	username := flags.String("username", "", "conta de operador ativa existente")
	backup := flags.Bool("backup-confirmed", false, "confirma backup recente e restaurável do snapshot store")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *username == "" || !*backup || flags.NArg() != 0 || os.Getenv("AUDITOR_DATABASE_URL") == "" {
		return errors.New("informe --username, --backup-confirmed e AUDITOR_DATABASE_URL")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("execute em um terminal interativo; senha por pipe não é aceita")
	}
	fmt.Fprint(os.Stderr, "Nova senha do operador (mínimo 16 caracteres): ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	defer clearBytes(first)
	fmt.Fprint(os.Stderr, "Repita a nova senha: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return err
	}
	defer clearBytes(second)
	if string(first) != string(second) {
		return errors.New("senhas diferentes")
	}
	hash, err := repository.HashAuditorPassword(string(first))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("AUDITOR_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := repository.NewStore(pool).RecoverOperatorPassword(ctx, *username, hash); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Senha redefinida, sessões revogadas e operação registrada. Entre novamente na aplicação.")
	return nil
}

func clearBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
